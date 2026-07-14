package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/tools"
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

type shellArgs struct {
	Command             string `json:"command" jsonschema:"required" jsonschema_description:"Shell command line, run by /bin/sh -c. Each call starts a fresh shell — cd, exported vars, and shell options do not persist between calls."`
	Description         string `json:"description,omitempty" jsonschema_description:"Short (5-10 word) active-voice summary of what this command does, shown in the UI. E.g. \"Run the test suite\", \"Install dependencies\"."`
	Timeout             int    `json:"timeout,omitempty" jsonschema_description:"Optional hard timeout in milliseconds; the command is killed when it elapses. 0 = no hard timeout."`
	RunInBackground     bool   `json:"run_in_background,omitempty" jsonschema_description:"Start the command in the background and return its shell id immediately, without waiting. Use for dev servers / watchers you intend to keep running."`
	AutoBackgroundAfter int    `json:"auto_background_after,omitempty" jsonschema_description:"Seconds a foreground command may run before it is automatically moved to the background and its shell id returned (default 60). Read the rest of its output with shell_output."`
}

func (a shellArgs) validate() error {
	if a.Command == "" {
		return errors.New("shell: command is required")
	}
	return nil
}

func (a shellArgs) timeout() time.Duration {
	return time.Duration(a.Timeout) * time.Millisecond
}

func (a shellArgs) autoBackgroundAfter() time.Duration {
	after := a.AutoBackgroundAfter
	if after <= 0 {
		after = defaultAutoBackgroundSeconds
	}
	return time.Duration(after) * time.Second
}

type shellOutputArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
	Block   bool   `json:"block,omitempty" jsonschema_description:"Block until the shell exits (or timeout elapses) before returning, instead of reading whatever output is available right now. Use to wait for a backgrounded command to finish — event-driven, so prefer it over a 'sleep' poll loop. Don't block on a process that never exits (e.g. a dev server) without a timeout."`
	Timeout int    `json:"timeout,omitempty" jsonschema_description:"With block, the longest to wait in milliseconds before returning the current output with a still-running status. Omit (or 0) to block until the command exits. Ignored without block."`
}

func (a shellOutputArgs) validate() error {
	if a.ShellID == "" {
		return errors.New("shell_output: shell_id is required")
	}
	return nil
}

type shellIDArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
}

func (a shellIDArgs) validate() error {
	if a.ShellID == "" {
		return errors.New("shell_kill: shell_id is required")
	}
	return nil
}

type toolSet struct {
	shells         *exec.Shells
	defaultWorkdir string
}

func Build(shells *exec.Shells, defaultWorkdir string) ([]tools.Tool, error) {
	if shells == nil {
		return nil, errors.New("shell: shells is nil")
	}
	t := &toolSet{shells: shells, defaultWorkdir: defaultWorkdir}

	shellTool, err := tools.New[shellArgs, string](
		tools.Config{
			Name: "shell",
			Description: "Execute a shell command via /bin/sh -c. Returns stdout/stderr, exit code, and duration. " +
				"Avoid `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, `read`, `edit` tools instead; reserve `shell` for operations that genuinely need a shell (build commands, git, package managers, etc.). " +
				"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls. " +
				"A command still running after auto_background_after seconds (default 60) is moved to the background and its shell id returned; read the rest of its output with shell_output and stop it with shell_kill. Set run_in_background to background it immediately.",
		},
		t.run,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build shell tool: %w", err)
	}
	outputTool, err := tools.New[shellOutputArgs, string](
		tools.Config{
			Name:        "shell_output",
			Description: "Read new output from a background shell (only output since the last read). Reports whether it is still running or has exited. With block, waits until the shell exits (or timeout ms) — wait for a backgrounded command without a sleep poll loop.",
		},
		t.output,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build shell_output tool: %w", err)
	}
	killTool, err := tools.New[shellIDArgs, string](
		tools.Config{
			Name:        "shell_kill",
			Description: "Stop a background shell.",
		},
		t.kill,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build shell_kill tool: %w", err)
	}
	return []tools.Tool{shellTool, outputTool, killTool}, nil
}

func (t *toolSet) run(ctx context.Context, a shellArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}

	id, err := t.shells.Launch(ctx, turnctx.TurnCwd(ctx, t.defaultWorkdir), a.Command, a.timeout())
	if err != nil {
		return "", err
	}
	if a.RunInBackground {
		return backgroundedJSON(id), nil
	}

	sh, ok := t.shells.Get(id)
	if !ok { // just launched — unreachable
		return "", fmt.Errorf("shell: background shell %s vanished", id)
	}
	timer := time.NewTimer(a.autoBackgroundAfter())
	defer timer.Stop()
	select {
	case <-sh.Done():
		return t.completed(id, sh), nil
	case <-timer.C:
		return backgroundedJSON(id), nil // still running — leave it
	case <-ctx.Done():
		return t.cancelForeground(ctx, id, sh)
	}
}

func (t *toolSet) completed(id string, sh *exec.Shell) string {
	out, dropped := sh.Read()
	code, killed, dur := sh.Outcome()
	t.shells.Remove(id)
	return completedJSON(out, dropped, code, killed, dur)
}

func (t *toolSet) cancelForeground(ctx context.Context, id string, sh *exec.Shell) (string, error) {
	// The command may have finished in the same instant the turn was canceled;
	// select picks a ready case at random, so check Done() before discarding a
	// completed result the user can still use.
	select {
	case <-sh.Done():
		return t.completed(id, sh), nil
	default:
		// Canceled mid-run: kill AND remove. A killed-and-discarded foreground
		// command is not a background job the model will query later, so leaving
		// it in the shell set just leaks a dead entry until engine shutdown.
		t.shells.Kill(id)
		t.shells.Remove(id)
		return "", ctx.Err()
	}
}

func (t *toolSet) output(ctx context.Context, a shellOutputArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}
	sh, ok := t.shells.Get(a.ShellID)
	if !ok {
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
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
	return fmt.Sprintf("Shell %s %s.\n%s", a.ShellID, state, string(b)), nil
}

func (t *toolSet) kill(_ context.Context, a shellIDArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}
	running, ok := t.shells.Kill(a.ShellID)
	switch {
	case !ok:
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	case running:
		return fmt.Sprintf("Killed background shell %s.", a.ShellID), nil
	default:
		return fmt.Sprintf("Background shell %s had already exited.", a.ShellID), nil
	}
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
