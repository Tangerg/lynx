package hooks

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
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

// CommandRequest is the shell-command work a hook adapter executes. The domain
// prepares stdin + timeout; the adapter owns how the command runs.
type CommandRequest struct {
	Command string
	Cwd     string
	Input   Input
	Timeout time.Duration
}

// CommandResult is the process-level outcome returned by the hook adapter.
type CommandResult struct {
	Decision CommandDecision
	Stderr   string
	ExitCode int
	Err      error
	TimedOut bool
}

// CommandVerdict is the closed control vocabulary returned by a hook command.
type CommandVerdict uint8

const (
	// CommandAllow is the zero-value verdict: a hook may inject or rewrite while
	// allowing the operation to continue.
	CommandAllow CommandVerdict = iota
	CommandDeny
	CommandAsk
)

// CommandDecision is the typed control information returned by a hook command.
// Its JSON process spelling belongs to the hook adapter, never this domain.
type CommandDecision struct {
	Verdict          CommandVerdict
	Reason           string
	InjectContext    string
	RewriteArguments string
}

// CommandRunner executes hook commands for the domain runner.
type CommandRunner interface {
	RunHookCommand(ctx context.Context, req CommandRequest) CommandResult
}

// Runner executes a trust-filtered hook set for one event and folds their
// outcomes into a single Decision.
type Runner struct {
	commands CommandRunner
	// onError, when set, is called for a hook that failed to run (spawn error,
	// timeout, or a non-blocking non-zero exit) so the caller can record it on
	// the turn's span (ctx carries it). nil = swallow. The hooks domain never
	// imports OTel; observability is the caller's, via this ctx-carrying hook.
	onError func(ctx context.Context, source string, err error)
}

// NewRunner builds a Runner. commands executes imperative hooks; onError may be
// nil. A nil commands runner means declarative inject hooks still work and
// command hooks degrade to non-blocking errors.
func NewRunner(commands CommandRunner, onError func(ctx context.Context, source string, err error)) *Runner {
	return &Runner{commands: commands, onError: onError}
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
			dec.fold(false, false, "", strings.TrimSpace(h.Inject), "")
			continue
		}
		r.runOne(ctx, h, in, &dec)
	}
	return dec
}

// runOne execs a single command hook and folds its outcome.
func (r *Runner) runOne(ctx context.Context, h Hook, in Input, dec *Decision) {
	if r.commands == nil {
		r.fail(ctx, h.Source, errors.New("hook command runner is not configured"))
		return
	}
	timeout := DefaultTimeout
	if h.TimeoutMs > 0 {
		timeout = time.Duration(h.TimeoutMs) * time.Millisecond
	}
	result := r.commands.RunHookCommand(ctx, CommandRequest{
		Command: h.Command,
		Cwd:     in.Cwd,
		Input:   in,
		Timeout: timeout,
	})
	if result.TimedOut {
		r.fail(ctx, h.Source, errors.New("hook timed out"))
		return
	}

	out := result.Decision

	switch {
	case result.Err == nil:
		// Exit 0: success. Apply any stdout-JSON decision (default allow).
		block := out.Verdict == CommandDeny
		ask := out.Verdict == CommandAsk
		reason := out.Reason
		if block && reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.fold(block, ask, reason, strings.TrimSpace(out.InjectContext), strings.TrimSpace(out.RewriteArguments))
	case result.ExitCode == blockExitCode:
		// Exit 2: block. Reason is the stdout JSON's, else stderr.
		reason := out.Reason
		if reason == "" {
			reason = strings.TrimSpace(result.Stderr)
		}
		dec.fold(true, false, reason, strings.TrimSpace(out.InjectContext), "")
	default:
		// Any other non-zero exit (or spawn failure): a broken hook. Non-blocking
		// — the action proceeds — but surfaced via onError so it's observable.
		r.fail(ctx, h.Source, hookError(result.ExitCode, result.Stderr, result.Err))
	}
}

func (r *Runner) fail(ctx context.Context, source string, err error) {
	if r.onError != nil {
		r.onError(ctx, source, err)
	}
}

// hookError builds a descriptive error for a non-blocking hook failure.
func hookError(exit int, stderr string, runErr error) error {
	if s := strings.TrimSpace(stderr); s != "" {
		return errors.New(s)
	}
	if runErr != nil {
		return runErr
	}
	return errors.New("hook exited with code " + strconv.Itoa(exit))
}

// Bound is the resolved hook set for one cwd, ready to fire events.
type Bound struct {
	hooks  []Hook
	runner *Runner
}

// NewBound binds a hook list to the runner that evaluates command hooks.
func NewBound(hooks []Hook, runner *Runner) *Bound {
	return &Bound{hooks: slices.Clone(hooks), runner: runner}
}

// Run fires the bound hooks for in's event and returns the combined Decision.
// Nil-safe: a nil Bound returns the zero Decision, so every seam can call
// st.hooks.Run(...) unguarded.
func (b *Bound) Run(ctx context.Context, in Input) Decision {
	if b == nil || b.runner == nil || len(b.hooks) == 0 {
		return Decision{}
	}
	return b.runner.Run(ctx, b.hooks, in)
}

// Empty reports whether the Bound has no hooks. Nil-safe.
func (b *Bound) Empty() bool { return b == nil || len(b.hooks) == 0 }

// Inspection is the read-only view of a cwd's hooks for a management surface
// (hooks.list): every discovered hook (trusted or not), the project
// root that gates the project-scope ones, and whether it's currently trusted.
type Inspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Hook
}
