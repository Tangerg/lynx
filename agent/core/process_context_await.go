package core

import "context"

// AwaitInput delegates to [Process.AwaitInput] — convenience because
// action code already has pc.
//
// It also records that this action invocation parked an awaitable, so a
// TYPED action (whose fn returns (Out, error) and can't return
// ActionWaiting directly) still suspends correctly: the typed-action
// wrapper checks [ProcessContext.InputAwaited] after the fn returns and
// reports ActionWaiting instead of writing the (unproduced) output.
// Untyped actions return this status directly and don't need the flag.
func (pc *ProcessContext) AwaitInput(ctx context.Context, req Awaitable) ActionStatus {
	if pc.Process == nil {
		return ActionFailed
	}
	status := pc.Process.AwaitInput(contextOrBackground(ctx), req)
	if status == ActionWaiting {
		pc.inputAwaited = true
	}
	return status
}

// InputAwaited reports whether this action invocation parked an
// awaitable via [ProcessContext.AwaitInput]. The typed-action wrapper
// uses it to translate "fn called AwaitInput" into ActionWaiting; the
// flag is per-invocation (ProcessContext is built fresh each tick).
func (pc *ProcessContext) InputAwaited() bool { return pc.inputAwaited }
