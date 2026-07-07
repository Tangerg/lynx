package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turnctx"
)

// Shell tools over a shared [exec.Shells]: the primary `shell` tool plus
// `shell_output` / `shell_kill` for the jobs it leaves running.
//
// Every command — foreground or explicitly backgrounded — starts as a detached
// job in that shell set. A foreground command races its completion against an
// auto-background window: finishing in time yields its output inline and the
// job is removed; outliving the window leaves it running, addressable by the
// same shell id, so shell_output / shell_kill work on it unchanged. This is the
// auto-background design — lyra selects on the per-shell done channel
// instead of polling. cwd is read per call (turnctx.TurnCwd) so a command runs in the
// session's working directory.

// defaultAutoBackgroundSeconds is how long a foreground shell command may run
// before it is moved to the background (so the turn isn't blocked on a build /
// dev server). Overridable per call via
// auto_background_after.
const defaultAutoBackgroundSeconds = 60

// The argument shapes double as the JSON schema source (via [pkgjson]) and the
// unmarshal target in each handler, so the parsed struct and the advertised
// schema can't drift.
type shellArgs struct {
	Command             string `json:"command" jsonschema:"required" jsonschema_description:"Shell command line, run by /bin/sh -c. Each call starts a fresh shell — cd, exported vars, and shell options do not persist between calls."`
	Description         string `json:"description,omitempty" jsonschema_description:"Short (5-10 word) active-voice summary of what this command does, shown in the UI. E.g. \"Run the test suite\", \"Install dependencies\"."`
	Timeout             int    `json:"timeout,omitempty" jsonschema_description:"Optional hard timeout in milliseconds; the command is killed when it elapses. 0 = no hard timeout."`
	RunInBackground     bool   `json:"run_in_background,omitempty" jsonschema_description:"Start the command in the background and return its shell id immediately, without waiting. Use for dev servers / watchers you intend to keep running."`
	AutoBackgroundAfter int    `json:"auto_background_after,omitempty" jsonschema_description:"Seconds a foreground command may run before it is automatically moved to the background and its shell id returned (default 60). Read the rest of its output with shell_output."`
}

type shellOutputArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
	Block   bool   `json:"block,omitempty" jsonschema_description:"Block until the shell exits (or timeout elapses) before returning, instead of reading whatever output is available right now. Use to wait for a backgrounded command to finish — event-driven, so prefer it over a 'sleep' poll loop. Don't block on a process that never exits (e.g. a dev server) without a timeout."`
	Timeout int    `json:"timeout,omitempty" jsonschema_description:"With block, the longest to wait in milliseconds before returning the current output with a still-running status. Omit (or 0) to block until the command exits. Ignored without block."`
}

type shellIDArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
}

var (
	shellSchema       = pkgjson.MustStringDefSchemaOf(shellArgs{})
	shellOutputSchema = pkgjson.MustStringDefSchemaOf(shellOutputArgs{})
	bgShellIDSchema   = pkgjson.MustStringDefSchemaOf(shellIDArgs{})
)

func Build(shells *exec.Shells, defaultWorkdir string) []chat.Tool {
	shell, _ := chat.NewTool(
		chat.ToolDefinition{
			Name: "shell",
			Description: "Execute a shell command via /bin/sh -c. Returns stdout/stderr, exit code, and duration. " +
				"Avoid `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, `read`, `edit` tools instead; reserve `shell` for operations that genuinely need a shell (build commands, git, package managers, etc.). " +
				"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls. " +
				"A command still running after auto_background_after seconds (default 60) is moved to the background and its shell id returned; read the rest of its output with shell_output and stop it with shell_kill. Set run_in_background to background it immediately.",
			InputSchema: shellSchema,
		},
		func(ctx context.Context, arguments string) (string, error) {
			var a shellArgs
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return "", fmt.Errorf("shell: invalid arguments: %w", err)
			}
			if a.Command == "" {
				return "", errors.New("shell: command is required")
			}

			id := shells.Launch(ctx, turnctx.TurnCwd(ctx, defaultWorkdir), a.Command, time.Duration(a.Timeout)*time.Millisecond)
			if a.RunInBackground {
				return backgroundedJSON(id), nil
			}

			sh, ok := shells.Get(id)
			if !ok { // just launched — unreachable
				return "", fmt.Errorf("shell: background shell %s vanished", id)
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
				shells.Remove(id)
				return completedJSON(out, dropped, code, killed, dur), nil
			case <-timer.C:
				return backgroundedJSON(id), nil // still running — leave it
			case <-ctx.Done():
				// The command may have finished in the same instant the turn was
				// canceled; select picks a ready case at random, so check Done()
				// before discarding a completed result the user can still use.
				select {
				case <-sh.Done():
					out, dropped := sh.Read()
					code, killed, dur := sh.Outcome()
					shells.Remove(id)
					return completedJSON(out, dropped, code, killed, dur), nil
				default:
					// Canceled mid-run: kill AND remove. A killed-and-discarded
					// foreground command is not a background job the model will
					// query later, so leaving it in the shell set (as the other
					// terminal paths Remove theirs) just leaks a dead entry until
					// engine shutdown.
					shells.Kill(id)
					shells.Remove(id)
					return "", ctx.Err()
				}
			}
		},
	)
	output, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "shell_output",
			Description: "Read new output from a background shell (only output since the last read). Reports whether it is still running or has exited. With block, waits until the shell exits (or timeout ms) — wait for a backgrounded command without a sleep poll loop.",
			InputSchema: shellOutputSchema,
		},
		func(ctx context.Context, arguments string) (string, error) {
			var a shellOutputArgs
			if err := json.Unmarshal([]byte(arguments), &a); err != nil {
				return "", fmt.Errorf("shell_output: invalid arguments: %w", err)
			}
			if a.ShellID == "" {
				return "", errors.New("shell_output: shell_id is required")
			}
			id := a.ShellID
			sh, ok := shells.Get(id)
			if !ok {
				return fmt.Sprintf("No background shell %s.", id), nil
			}
			if a.Block {
				if err := waitForShell(ctx, sh, a.Timeout); err != nil {
					return "", err
				}
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
			Name:        "shell_kill",
			Description: "Stop a background shell.",
			InputSchema: bgShellIDSchema,
		},
		func(_ context.Context, arguments string) (string, error) {
			id, err := bgShellID(arguments, "shell_kill")
			if err != nil {
				return "", err
			}
			running, ok := shells.Kill(id)
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
	return []chat.Tool{shell, output, kill}
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
		"Command running in background as shell %s. Read its output with shell_output {\"shell_id\":%q} and stop it with shell_kill.",
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

// waitForShell blocks until sh exits, ctx is canceled, or — when timeoutMs > 0
// — the timeout elapses. It reuses the same per-shell done channel the shell
// foreground path selects on (no polling). A timeout is NOT an error: the
// caller then reports the current still-running output, just as if block were
// off. Returns ctx.Err() only on cancellation (turn cancel / budget timeout).
func waitForShell(ctx context.Context, sh *exec.Shell, timeoutMs int) error {
	if timeoutMs <= 0 {
		select {
		case <-sh.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-sh.Done():
	case <-timer.C: // still running — fall through to report current state
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
