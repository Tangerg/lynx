package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

// bgShellManager runs shell commands in the background and lets the model poll
// their output and stop them — the long-running-command counterpart to the
// synchronous bash tool. The process handles + output buffers live here, in
// the engine, so the shared (stateless) tools/bash module stays untouched.
//
// No PTY (matching claude_code / opencode; only codex uses one, at a heavy
// platform cost): plain pipes into a bounded ring buffer, read incrementally.
// Cross-platform with no platform-specific features — kill is a plain process
// kill (same /bin/sh -c shell + same reach as the synchronous bash tool), so a
// command that itself forks grandchildren may leave them, which is acceptable
// for the local single-user runtime.
type bgShellManager struct {
	mu     sync.Mutex
	nextID int
	shells map[string]*bgShell
}

func newBgShellManager() *bgShellManager {
	return &bgShellManager{shells: map[string]*bgShell{}}
}

// maxBgBuffer caps a background shell's retained output; once exceeded the
// oldest bytes are dropped (a poll that fell behind is told output was lost).
const maxBgBuffer = 256 * 1024

type bgShell struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd

	mu       sync.Mutex
	buf      []byte // tail of stdout+stderr, capped at maxBgBuffer
	total    int    // absolute bytes ever written (buf holds the last len(buf))
	readPos  int    // absolute offset already returned to the model
	done     bool
	exitInfo string // "exit 0" / "exit 2" / "signal: killed" — set on completion
}

// launch starts command under cwd in the background, detached from the
// tool-call context so it outlives the turn, and returns its shell id.
func (m *bgShellManager) launch(cwd, command string) string {
	runCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	sh := &bgShell{cancel: cancel, cmd: cmd}
	cmd.Stdout = sh
	cmd.Stderr = sh

	m.mu.Lock()
	m.nextID++
	id := "bg_" + strconv.Itoa(m.nextID)
	m.shells[id] = sh
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		sh.finish("start failed: " + err.Error())
		return id
	}
	go func() {
		err := cmd.Wait()
		cancel()
		if err != nil {
			sh.finish(err.Error())
		} else {
			sh.finish("exit 0")
		}
	}()
	return id
}

func (m *bgShellManager) get(id string) (*bgShell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sh, ok := m.shells[id]
	return sh, ok
}

// kill stops a background shell; reports whether it was still running.
func (m *bgShellManager) kill(id string) (bool, bool) {
	sh, ok := m.get(id)
	if !ok {
		return false, false
	}
	sh.mu.Lock()
	running := !sh.done
	sh.mu.Unlock()
	if running {
		sh.cancel()
		_ = sh.cmd.Process.Kill()
	}
	return running, true
}

// killAll stops every background shell — called on engine shutdown.
func (m *bgShellManager) killAll() {
	m.mu.Lock()
	shells := m.shells
	m.shells = map[string]*bgShell{}
	m.mu.Unlock()
	for _, sh := range shells {
		sh.cancel()
		if sh.cmd.Process != nil {
			_ = sh.cmd.Process.Kill()
		}
	}
}

func (s *bgShell) finish(info string) {
	s.mu.Lock()
	s.done = true
	s.exitInfo = info
	s.mu.Unlock()
}

// read returns the output not yet returned to the model and whether earlier
// output had to be dropped (the buffer overflowed before this poll).
func (s *bgShell) read() (out string, dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bufStart := s.total - len(s.buf)
	if s.readPos < bufStart {
		dropped = true
		s.readPos = bufStart
	}
	out = string(s.buf[s.readPos-bufStart:])
	s.readPos = s.total
	return out, dropped
}

func (s *bgShell) status() (done bool, info string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done, s.exitInfo
}

// Write funnels the shell's stdout/stderr into its capped ring buffer (the
// process's Stdout/Stderr point straight at the bgShell). On overflow the
// oldest bytes are dropped — a poll that fell behind learns so via read.
func (s *bgShell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += len(p)
	s.buf = append(s.buf, p...)
	if len(s.buf) > maxBgBuffer {
		s.buf = s.buf[len(s.buf)-maxBgBuffer:]
	}
	return len(p), nil
}

// --- tools ---

const (
	bgRunSchema    = `{"type":"object","properties":{"command":{"type":"string","description":"Shell command line, run by /bin/sh -c in the background."}},"required":["command"]}`
	bgShellIDSchema = `{"type":"object","properties":{"shell_id":{"type":"string","description":"Background shell id returned by run_in_background."}},"required":["shell_id"]}`
)

// buildBgShellTools builds the background-command tools over a shared manager.
// cwd is read per call (turnCwd) so a background command runs in the session's
// working directory, like the synchronous bash tool.
func buildBgShellTools(mgr *bgShellManager, defaultWorkdir string) []chat.Tool {
	run, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "run_in_background",
			Description: "Start a shell command in the background and return its shell id immediately. Use for long-running commands (dev servers, watchers, builds) so the turn isn't blocked. Read its output with bash_output and stop it with kill_shell.",
			InputSchema: bgRunSchema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var a struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return "", fmt.Errorf("run_in_background: invalid arguments: %w", err)
			}
			if a.Command == "" {
				return "", errors.New("run_in_background: command is required")
			}
			id := mgr.launch(turnCwd(ctx, defaultWorkdir), a.Command)
			return fmt.Sprintf("Started background shell %s. Use bash_output {\"shell_id\":%q} to read output, kill_shell to stop it.", id, id), nil
		},
	)
	output, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "bash_output",
			Description: "Read new output from a background shell started with run_in_background (only output since the last read). Reports whether it is still running or has exited.",
			InputSchema: bgShellIDSchema,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			id, err := bgShellID(arguments, "bash_output")
			if err != nil {
				return "", err
			}
			sh, ok := mgr.get(id)
			if !ok {
				return fmt.Sprintf("No background shell %s.", id), nil
			}
			out, dropped := sh.read()
			done, info := sh.status()
			state := "still running"
			if done {
				state = "finished (" + info + ")"
			}
			var b []byte
			if dropped {
				b = append(b, "[earlier output dropped — buffer overflowed]\n"...)
			}
			b = append(b, out...)
			return fmt.Sprintf("Shell %s %s.\n%s", id, state, string(b)), nil
		},
	)
	kill, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "kill_shell",
			Description: "Stop a background shell started with run_in_background.",
			InputSchema: bgShellIDSchema,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			id, err := bgShellID(arguments, "kill_shell")
			if err != nil {
				return "", err
			}
			running, ok := mgr.kill(id)
			switch {
			case !ok:
				return fmt.Sprintf("No background shell %s.", id), nil
			case running:
				return fmt.Sprintf("Killed background shell %s.", id), nil
			default:
				return fmt.Sprintf("Background shell %s had already exited.", id), nil
			}
		},
	)
	return []chat.Tool{run, output, kill}
}

func bgShellID(arguments, tool string) (string, error) {
	var a struct {
		ShellID string `json:"shell_id"`
	}
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return "", fmt.Errorf("%s: invalid arguments: %w", tool, err)
	}
	if a.ShellID == "" {
		return "", fmt.Errorf("%s: shell_id is required", tool)
	}
	return a.ShellID, nil
}
