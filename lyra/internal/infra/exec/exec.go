// Package exec is the background-process mechanism: it runs shell commands
// detached from the calling turn, buffers their output in a bounded ring so
// the model can poll incrementally, and kills them on demand. It is pure
// infra — no domain knowledge, no upward dependency. The engine builds the
// run_in_background / bash_output / kill_shell tools over a [Manager]; the
// synchronous bash tool (tools/bash) is unaffected.
//
// No PTY (matching claude_code / opencode; only codex uses one, at a heavy
// platform cost): plain pipes into a bounded ring buffer, read incrementally.
// Cross-platform with no platform-specific features — kill is a plain process
// kill (same /bin/sh -c shell + reach as the synchronous bash tool), so a
// command that itself forks grandchildren may leave them, acceptable for the
// local single-user runtime.
package exec

import (
	"context"
	"os/exec"
	"strconv"
	"sync"
)

// maxBuffer caps a background shell's retained output; once exceeded the
// oldest bytes are dropped (a poll that fell behind is told output was lost).
const maxBuffer = 256 * 1024

// Manager runs shell commands in the background and lets callers poll their
// output and stop them — the long-running-command counterpart to a synchronous
// bash executor. The process handles + output buffers live here. The zero
// value is not usable; build one with [NewManager].
type Manager struct {
	mu     sync.Mutex
	nextID int
	shells map[string]*Shell
}

func NewManager() *Manager {
	return &Manager{shells: map[string]*Shell{}}
}

// Shell is one background process: its handle, the tail of its combined
// stdout+stderr (capped), and its completion state. Poll it with [Shell.Read]
// / [Shell.Status]; the [Manager] owns its lifecycle.
type Shell struct {
	cancel context.CancelFunc
	cmd    *exec.Cmd

	mu       sync.Mutex
	buf      []byte // tail of stdout+stderr, capped at maxBuffer
	total    int    // absolute bytes ever written (buf holds the last len(buf))
	readPos  int    // absolute offset already returned to the caller
	done     bool
	exitInfo string // "exit 0" / "exit 2" / "signal: killed" — set on completion
}

// Launch starts command under cwd in the background, detached from any
// tool-call context so it outlives the turn, and returns its shell id.
func (m *Manager) Launch(cwd, command string) string {
	runCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	sh := &Shell{cancel: cancel, cmd: cmd}
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

// Get returns the shell with id and whether it exists.
func (m *Manager) Get(id string) (*Shell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sh, ok := m.shells[id]
	return sh, ok
}

// Kill stops a background shell; reports whether it was still running and
// whether it existed.
func (m *Manager) Kill(id string) (running, ok bool) {
	sh, ok := m.Get(id)
	if !ok {
		return false, false
	}
	sh.mu.Lock()
	running = !sh.done
	sh.mu.Unlock()
	if running {
		sh.cancel()
		_ = sh.cmd.Process.Kill()
	}
	return running, true
}

// KillAll stops every background shell — called on engine shutdown.
func (m *Manager) KillAll() {
	m.mu.Lock()
	shells := m.shells
	m.shells = map[string]*Shell{}
	m.mu.Unlock()
	for _, sh := range shells {
		sh.cancel()
		if sh.cmd.Process != nil {
			_ = sh.cmd.Process.Kill()
		}
	}
}

func (s *Shell) finish(info string) {
	s.mu.Lock()
	s.done = true
	s.exitInfo = info
	s.mu.Unlock()
}

// Read returns the output not yet returned to the caller and whether earlier
// output had to be dropped (the buffer overflowed before this poll).
func (s *Shell) Read() (out string, dropped bool) {
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

// Status reports whether the shell finished and its exit info.
func (s *Shell) Status() (done bool, info string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done, s.exitInfo
}

// Write funnels the shell's stdout/stderr into its capped ring buffer (the
// process's Stdout/Stderr point straight at the Shell). On overflow the oldest
// bytes are dropped — a poll that fell behind learns so via [Shell.Read].
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += len(p)
	s.buf = append(s.buf, p...)
	if len(s.buf) > maxBuffer {
		s.buf = s.buf[len(s.buf)-maxBuffer:]
	}
	return len(p), nil
}
