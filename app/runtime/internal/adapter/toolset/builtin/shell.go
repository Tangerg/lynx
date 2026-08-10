package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
)

// Shell tools over a shared [exec.Shells]: the primary `shell` tool plus
// `read_shell_output` / `stop_shell` for the jobs it leaves running.
//
// Every command — foreground or explicitly backgrounded — starts as a detached
// job in that shell set. A foreground command races its completion against an
// auto-background window: finishing in time yields its output inline and the
// job is removed; outliving the window leaves it running, addressable by the
// same shell id, so the lifecycle tools work on it unchanged. This is the
// auto-background design — lyra selects on the per-shell done channel
// instead of polling. cwd is read per call (executionctx.CWD) so a command runs in the
// session's working directory.

// defaultAutoBackgroundSeconds is how long a foreground shell command may run
// before it is moved to the background (so the Run isn't blocked on a build /
// dev server). Overridable per call via
// auto_background_after_seconds.
const defaultAutoBackgroundSeconds = 60

type shellArgs struct {
	Command                    string `json:"command" jsonschema:"minLength=1" jsonschema_description:"Shell command line, run by /bin/sh -c. Each call starts a fresh shell; directory changes, variables, and shell options do not persist."`
	Description                string `json:"description" jsonschema:"minLength=1,maxLength=120" jsonschema_description:"Concise action phrase shown while the command runs, such as Run backend tests. Describe the command's purpose; do not copy the command or predict its result."`
	TimeoutMillis              int    `json:"timeout_millis,omitempty" jsonschema:"minimum=1" jsonschema_description:"Hard execution timeout in milliseconds. Omit for no hard timeout."`
	RunInBackground            bool   `json:"run_in_background,omitempty" jsonschema_description:"Return immediately with a shell_id while the command keeps running. Use for servers and watchers."`
	AutoBackgroundAfterSeconds int    `json:"auto_background_after_seconds,omitempty" jsonschema:"minimum=1" jsonschema_description:"Move a foreground command to the background after this many seconds. Defaults to 60."`
}

func (a shellArgs) validate() error {
	if a.Command == "" {
		return errors.New("shell: command is required")
	}
	if strings.TrimSpace(a.Description) == "" {
		return errors.New("shell: description is required")
	}
	if strings.TrimSpace(a.Description) != a.Description {
		return errors.New("shell: description must not have surrounding whitespace")
	}
	if a.RunInBackground && a.AutoBackgroundAfterSeconds > 0 {
		return errors.New("shell: auto_background_after_seconds cannot be used when run_in_background=true")
	}
	return nil
}

func (a shellArgs) timeout() time.Duration {
	return time.Duration(a.TimeoutMillis) * time.Millisecond
}

func (a shellArgs) autoBackgroundAfter() time.Duration {
	after := a.AutoBackgroundAfterSeconds
	if after <= 0 {
		after = defaultAutoBackgroundSeconds
	}
	return time.Duration(after) * time.Second
}

type shellOutputArgs struct {
	ShellID       string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
	Wait          bool   `json:"wait,omitempty" jsonschema_description:"Wait for the shell to exit before returning new output. Use this instead of sleep polling; avoid waiting indefinitely on a server or watcher."`
	TimeoutMillis int    `json:"timeout_millis,omitempty" jsonschema:"minimum=1" jsonschema_description:"When wait=true, maximum milliseconds to wait before returning current output. Omit to wait until exit. Do not pass when wait=false."`
}

func (a shellOutputArgs) validate() error {
	if a.ShellID == "" {
		return errors.New("read_shell_output: shell_id is required")
	}
	if !a.Wait && a.TimeoutMillis > 0 {
		return errors.New("read_shell_output: timeout_millis requires wait=true")
	}
	return nil
}

type shellIDArgs struct {
	ShellID string `json:"shell_id" jsonschema:"required" jsonschema_description:"Background shell id returned by shell when a long-running command was moved to the background."`
}

func (a shellIDArgs) validate() error {
	if a.ShellID == "" {
		return errors.New("stop_shell: shell_id is required")
	}
	return nil
}

type commandTools struct {
	shells     *exec.Shells
	defaultCWD string
}

func BuildShell(shells *exec.Shells, defaultCWD string) ([]toolcontract.Tool, error) {
	if shells == nil {
		return nil, errors.New("shell: shells is nil")
	}
	t := &commandTools{shells: shells, defaultCWD: defaultCWD}

	shellTool, err := toolcontract.NewFunc[shellArgs, string](
		toolcontract.FuncConfig{
			Name: toolname.Shell,
			Description: "Execute a shell command via /bin/sh -c. Returns stdout/stderr, exit code, and duration. " +
				"Set description to a concise action label that explains the command's purpose while it runs. " +
				"Avoid `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, and `read` tools instead; use `apply_patch` for file changes. Reserve `shell` for operations that genuinely need a shell (build commands, git, package managers, etc.). " +
				"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls. " +
				"A command still running after auto_background_after_seconds (default 60) is moved to the background; continue with read_shell_output or stop_shell. Set run_in_background to background it immediately.",
		},
		t.run,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build shell tool: %w", err)
	}
	outputTool, err := toolcontract.NewFunc[shellOutputArgs, string](
		toolcontract.FuncConfig{
			Name:        toolname.ReadShellOutput,
			Description: "Read only the new output produced by a background shell since the previous read and report whether it is still running. Set wait=true to wait event-first for exit instead of sleep polling; bound that wait with timeout_millis for servers or watchers.",
		},
		t.output,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build read_shell_output tool: %w", err)
	}
	killTool, err := toolcontract.NewFunc[shellIDArgs, string](
		toolcontract.FuncConfig{
			Name:        toolname.StopShell,
			Description: "Stop one background shell by the shell_id returned from shell.",
		},
		t.kill,
	)
	if err != nil {
		return nil, fmt.Errorf("shell: build stop_shell tool: %w", err)
	}
	return []toolcontract.Tool{shellTool, outputTool, killTool}, nil
}

func (t *commandTools) run(ctx context.Context, a shellArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}

	id, err := t.shells.Launch(ctx, executionctx.SessionID(ctx), executionctx.CWD(ctx, t.defaultCWD), a.Command, a.timeout(), executionctx.Isolated(ctx))
	if err != nil {
		return "", err
	}
	if a.RunInBackground {
		return backgroundedJSON(id)
	}

	sh, ok := t.shells.Get(id)
	if !ok { // just launched — unreachable
		return "", fmt.Errorf("shell: background shell %s vanished", id)
	}
	timer := time.NewTimer(a.autoBackgroundAfter())
	defer timer.Stop()
	select {
	case <-sh.Done():
		return t.completed(id, sh)
	case <-timer.C:
		return backgroundedJSON(id) // still running — leave it
	case <-ctx.Done():
		return t.cancelForeground(ctx, id, sh)
	}
}

func (t *commandTools) completed(id string, sh *exec.Shell) (string, error) {
	out, dropped := sh.Read()
	code, killed, dur := sh.Outcome()
	t.shells.Remove(id)
	return completedJSON(out, dropped, code, killed, dur)
}

func (t *commandTools) cancelForeground(ctx context.Context, id string, sh *exec.Shell) (string, error) {
	// The command may have finished in the same instant the Run was canceled;
	// select picks a ready case at random, so check Done() before discarding a
	// completed result the user can still use.
	select {
	case <-sh.Done():
		return t.completed(id, sh)
	default:
		// Canceled mid-run: kill AND remove. A killed-and-discarded foreground
		// command is not a background job the model will query later, so leaving
		// it in the shell set just leaks a dead entry until engine shutdown.
		if _, err := t.shells.Kill(id); err != nil && !errors.Is(err, exec.ErrShellNotFound) {
			return "", errors.Join(ctx.Err(), fmt.Errorf("shell: stop canceled foreground command %q: %w", id, err))
		}
		t.shells.Remove(id)
		return "", ctx.Err()
	}
}

func (t *commandTools) output(ctx context.Context, a shellOutputArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}
	sh, ok := t.shells.Get(a.ShellID)
	if !ok {
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	}
	if a.Wait {
		if err := waitForShell(ctx, sh, a.TimeoutMillis); err != nil {
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

func (t *commandTools) kill(_ context.Context, a shellIDArgs) (string, error) {
	if err := a.validate(); err != nil {
		return "", err
	}
	running, err := t.shells.Kill(a.ShellID)
	switch {
	case errors.Is(err, exec.ErrShellNotFound):
		return fmt.Sprintf("No background shell %s.", a.ShellID), nil
	case err != nil:
		return "", fmt.Errorf("shell: kill background shell %q: %w", a.ShellID, err)
	case running:
		return fmt.Sprintf("Killed background shell %s.", a.ShellID), nil
	default:
		return fmt.Sprintf("Background shell %s had already exited.", a.ShellID), nil
	}
}

// completedJSON shapes a finished foreground command's result. The combined
// stdout+stderr goes in "stdout" because the execution ring preserves their
// combined arrival order. exit_code is always present for a finished command.
func completedJSON(out string, dropped bool, code int, killed bool, dur time.Duration) (string, error) {
	if dropped {
		out = "[earlier output dropped — buffer overflowed]\n" + out
	}
	b, err := json.Marshal(struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
		Killed   bool   `json:"killed,omitempty"`
		Duration string `json:"duration"`
	}{Stdout: out, ExitCode: code, Killed: killed, Duration: dur.String()})
	if err != nil {
		return "", fmt.Errorf("shell: encode completed command result: %w", err)
	}
	return string(b), nil
}

// backgroundedJSON is the result for a command left running (explicit
// run_in_background or auto-backgrounded). It omits exit_code — the command
// has not exited and therefore has no exit status.
func backgroundedJSON(id string) (string, error) {
	b, err := json.Marshal(struct {
		Stdout string `json:"stdout"`
	}{Stdout: fmt.Sprintf(
		"Command running in background as shell %s. Continue with read_shell_output {\"shell_id\":%q} or stop_shell {\"shell_id\":%q}.",
		id, id, id)})
	if err != nil {
		return "", fmt.Errorf("shell: encode background command result: %w", err)
	}
	return string(b), nil
}

// waitForShell blocks until sh exits, ctx is canceled, or — when timeoutMillis > 0
// — the timeout elapses. It reuses the same per-shell done channel the shell
// foreground path selects on (no polling). A timeout is NOT an error: the
// caller then reports the current still-running output, just as if wait were
// off. Returns ctx.Err() only on cancellation (Run cancel / budget timeout).
func waitForShell(ctx context.Context, sh *exec.Shell, timeoutMillis int) error {
	if timeoutMillis <= 0 {
		select {
		case <-sh.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	timer := time.NewTimer(time.Duration(timeoutMillis) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-sh.Done():
	case <-timer.C: // still running — fall through to report current state
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
