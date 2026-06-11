package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/infra/exec"
)

// Background-command tools (run_in_background / bash_output / kill_shell) over
// a shared [exec.Manager]. The process mechanism — spawn, the bounded output
// ring, kill — lives in infra/exec; this file is just the chat.Tool assembly
// that exposes it to the model (the long-running counterpart to the
// synchronous bash tool). cwd is read per call (turnCwd) so a background
// command runs in the session's working directory, like the bash tool.

const (
	bgRunSchema     = `{"type":"object","properties":{"command":{"type":"string","description":"Shell command line, run by /bin/sh -c in the background."}},"required":["command"]}`
	bgShellIDSchema = `{"type":"object","properties":{"shell_id":{"type":"string","description":"Background shell id returned by run_in_background."}},"required":["shell_id"]}`
)

func buildBgShellTools(mgr *exec.Manager, defaultWorkdir string) []chat.Tool {
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
			id := mgr.Launch(turnCwd(ctx, defaultWorkdir), a.Command)
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
			Description: "Stop a background shell started with run_in_background.",
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
