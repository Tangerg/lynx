package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/engine/toolset/turnctx"
	"github.com/Tangerg/lynx/lyra/internal/infra/exec"
)

// Shell tools over a shared [exec.Manager]: the primary `bash` tool plus
// `bash_output` / `kill_shell` for the jobs it leaves running.
//
// Every command — foreground or explicitly backgrounded — starts as a detached
// job on the manager. A foreground command races its completion against an
// auto-background window: finishing in time yields its output inline and the
// job is removed; outliving the window leaves it running, addressable by the
// same shell id, so bash_output / kill_shell work on it unchanged. This is the
// crush auto-background design — lyra selects on the per-shell done channel
// instead of polling. cwd is read per call (turnctx.TurnCwd) so a command runs in the
// session's working directory.

// defaultAutoBackgroundSeconds is how long a foreground bash command may run
// before it is moved to the background (so the turn isn't blocked on a build /
// dev server). Mirrors crush's 60s default; overridable per call via
// auto_background_after.
const defaultAutoBackgroundSeconds = 60

const (
	bashSchema = `{"type":"object","properties":{` +
		`"command":{"type":"string","description":"Shell command line, run by /bin/sh -c. Each call starts a fresh shell — cd, exported vars, and shell options do not persist between calls."},` +
		`"timeout":{"type":"integer","description":"Optional hard timeout in milliseconds; the command is killed when it elapses. 0 = no hard timeout."},` +
		`"run_in_background":{"type":"boolean","description":"Start the command in the background and return its shell id immediately, without waiting. Use for dev servers / watchers you intend to keep running."},` +
		`"auto_background_after":{"type":"integer","description":"Seconds a foreground command may run before it is automatically moved to the background and its shell id returned (default 60). Read the rest of its output with bash_output."}` +
		`},"required":["command"]}`
	bgShellIDSchema = `{"type":"object","properties":{"shell_id":{"type":"string","description":"Background shell id returned by bash when a long-running command was moved to the background."}},"required":["shell_id"]}`
)

func Build(mgr *exec.Manager, defaultWorkdir string) []chat.Tool {
	bash, _ := chat.NewTool(
		chat.ToolDefinition{
			Name: "bash",
			Description: "Execute a shell command via /bin/sh -c. Returns stdout/stderr, exit code, and duration. " +
				"Avoid `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, `read`, `edit` tools instead; reserve bash for shell-only operations (build commands, git, package managers, etc.). " +
				"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls. " +
				"A command still running after auto_background_after seconds (default 60) is moved to the background and its shell id returned; read the rest of its output with bash_output and stop it with kill_shell. Set run_in_background to background it immediately.",
			InputSchema: bashSchema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var a struct {
				Command             string `json:"command"`
				Timeout             int    `json:"timeout"`
				RunInBackground     bool   `json:"run_in_background"`
				AutoBackgroundAfter int    `json:"auto_background_after"`
			}
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return "", fmt.Errorf("bash: invalid arguments: %w", err)
			}
			if a.Command == "" {
				return "", errors.New("bash: command is required")
			}

			id := mgr.Launch(turnctx.TurnCwd(ctx, defaultWorkdir), a.Command, time.Duration(a.Timeout)*time.Millisecond)
			if a.RunInBackground {
				return backgroundedJSON(id), nil
			}

			sh, ok := mgr.Get(id)
			if !ok { // just launched — unreachable
				return "", fmt.Errorf("bash: shell %s vanished", id)
			}
			after := a.AutoBackgroundAfter
			if after <= 0 {
				after = defaultAutoBackgroundSeconds
			}
			timer := time.NewTimer(time.Duration(after) * time.Second)
			defer timer.Stop()
			select {
			case <-sh.Done():
				// Completed within the window: readPos is still 0, so one Read
				// drains the whole retained output. Remove it — not a background job.
				out, dropped := sh.Read()
				code, killed, dur := sh.Outcome()
				mgr.Remove(id)
				return completedJSON(out, dropped, code, killed, dur), nil
			case <-timer.C:
				return backgroundedJSON(id), nil // still running — leave it
			case <-ctx.Done():
				mgr.Kill(id)
				return "", ctx.Err()
			}
		},
	)
	output, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "bash_output",
			Description: "Read new output from a background shell (only output since the last read). Reports whether it is still running or has exited.",
			InputSchema: bgShellIDSchema,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			id, err := bgShellID(arguments, "bash_output")
			if err != nil {
				return "", err
			}
			sh, ok := mgr.Get(id)
			if !ok {
				return fmt.Sprintf("No background shell %s.", id), nil
			}
			out, dropped := sh.Read()
			done, info := sh.Status()
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
			Description: "Stop a background shell.",
			InputSchema: bgShellIDSchema,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			id, err := bgShellID(arguments, "kill_shell")
			if err != nil {
				return "", err
			}
			running, ok := mgr.Kill(id)
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
	return []chat.Tool{bash, output, kill}
}

// completedJSON shapes a finished foreground command's result. The combined
// stdout+stderr goes in "stdout" (the exec ring merges the two streams; the
// server's commandResult merges them on the wire anyway). exit_code is always
// present, so the client renders it.
func completedJSON(out string, dropped bool, code int, killed bool, dur time.Duration) string {
	if dropped {
		out = "[earlier output dropped — buffer overflowed]\n" + out
	}
	b, _ := json.Marshal(struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
		Killed   bool   `json:"killed,omitempty"`
		Duration string `json:"duration"`
	}{Stdout: out, ExitCode: code, Killed: killed, Duration: dur.String()})
	return string(b)
}

// backgroundedJSON is the result for a command left running (explicit
// run_in_background or auto-backgrounded). It omits exit_code — the command
// hasn't exited — so the server's commandResult renders no phantom "exit 0".
func backgroundedJSON(id string) string {
	b, _ := json.Marshal(struct {
		Stdout string `json:"stdout"`
	}{Stdout: fmt.Sprintf(
		"Command running in background as shell %s. Read its output with bash_output {\"shell_id\":%q} and stop it with kill_shell.",
		id, id)})
	return string(b)
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
