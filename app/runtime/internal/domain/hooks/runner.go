package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"time"
)

// DefaultTimeout bounds a single hook command — a hook must not wedge the turn
// it gates. A hook that exceeds it is killed and treated as a non-blocking error
// (the action proceeds), so a hung hook degrades to "no hook" rather than a
// frozen agent.
const DefaultTimeout = 30 * time.Second

// blockExitCode is the one non-zero exit a hook uses to BLOCK the action
// (stderr becomes the reason fed to the model). Every other non-zero exit is a
// non-blocking error: the hook is broken, but a broken hook must not brick the
// session, so the action proceeds. Explicit deny is the only hard stop — same
// contract as Claude Code.
const blockExitCode = 2

// Runner executes a set of hooks for one event and folds their outcomes into a
// single Decision. It's the pure execution core: it's handed the (already
// trust-filtered) hook list — discovery + trust gating happen above it.
type Runner struct {
	// onError, when set, is called for a hook that failed to run (spawn error,
	// timeout, or a non-blocking non-zero exit) so the caller can record it on a
	// span. nil = swallow. The hooks domain never logs directly (observability is
	// the caller's span).
	onError func(source string, err error)
}

// NewRunner builds a Runner. onError may be nil.
func NewRunner(onError func(source string, err error)) *Runner {
	return &Runner{onError: onError}
}

// Run fires every hook matching in's event (and, for tool events, its tool
// name) and returns the combined Decision. A declarative `inject` hook
// contributes context with no process spawn; a `command` hook is exec'd with in
// as JSON on stdin. Hooks run in list order (loader order: global before
// project), so the combine rules (first-block-wins, first-rewrite-wins) are
// deterministic.
func (r *Runner) Run(ctx context.Context, hooks []Hook, in Input) Decision {
	var dec Decision
	for _, h := range hooks {
		if !h.matches(in) {
			continue
		}
		if h.Command == "" {
			// Declarative: a literal context injection, no exec.
			dec.fold(false, false, "", trimZero(h.Inject), "")
			continue
		}
		r.runOne(ctx, h, in, &dec)
	}
	return dec
}

// runOne execs a single command hook and folds its outcome.
func (r *Runner) runOne(ctx context.Context, h Hook, in Input, dec *Decision) {
	stdin, err := json.Marshal(in)
	if err != nil {
		r.fail(h.Source, err)
		return
	}
	timeout := DefaultTimeout
	if h.TimeoutMs > 0 {
		timeout = time.Duration(h.TimeoutMs) * time.Millisecond
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// sh -c so a hook can be a one-liner or a script path with args, like the
	// shell the user already configures the bash tool with.
	cmd := exec.CommandContext(cctx, "sh", "-c", h.Command)
	cmd.Stdin = bytes.NewReader(stdin)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cctx.Err() == context.DeadlineExceeded {
		r.fail(h.Source, errors.New("hook timed out"))
		return
	}

	out := parseOutput(stdout.Bytes())
	exit := exitCodeOf(runErr)

	switch {
	case runErr == nil:
		// Exit 0: success. Apply any stdout-JSON decision (default allow).
		block := out.Decision == "deny"
		ask := out.Decision == "ask"
		reason := out.Reason
		if block && reason == "" {
			reason = trimZero(stderr.String())
		}
		dec.fold(block, ask, reason, trimZero(out.InjectContext), trimZero(out.RewriteArguments))
	case exit == blockExitCode:
		// Exit 2: block. Reason is the stdout JSON's, else stderr.
		reason := out.Reason
		if reason == "" {
			reason = trimZero(stderr.String())
		}
		dec.fold(true, false, reason, trimZero(out.InjectContext), "")
	default:
		// Any other non-zero exit (or spawn failure): a broken hook. Non-blocking
		// — the action proceeds — but surfaced via onError so it's observable.
		r.fail(h.Source, hookError(exit, stderr.String(), runErr))
	}
}

func (r *Runner) fail(source string, err error) {
	if r.onError != nil {
		r.onError(source, err)
	}
}

// parseOutput decodes a hook's stdout as the control JSON; a non-JSON / empty
// stdout yields a zero hookOutput (exit code alone decides).
func parseOutput(b []byte) hookOutput {
	var out hookOutput
	if len(bytes.TrimSpace(b)) == 0 {
		return out
	}
	_ = json.Unmarshal(b, &out) // best-effort: non-JSON stdout → exit-code-only
	return out
}

// exitCodeOf extracts the process exit code, or -1 when the command never ran
// (spawn failure) or was killed without a code.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}

// hookError builds a descriptive error for a non-blocking hook failure.
func hookError(exit int, stderr string, runErr error) error {
	if s := trimZero(stderr); s != "" {
		return errors.New(s)
	}
	if runErr != nil {
		return runErr
	}
	return errors.New("hook exited with code " + strconv.Itoa(exit))
}
