package hitl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// PauseError is the sentinel error returned by HITL-decorated tools
// when the agent process must be suspended to gather user input.
//
// The error surfaces through [chat.NewToolMiddleware] as the failure
// from the LLM call (the tool's Call returned an error, so the
// middleware bails and propagates). Action bodies detect it via
// errors.As and route the carried [core.Awaitable] into
// [core.ProcessContext.AwaitInput].
//
// Use [HandlePause] for the canonical handling pattern — it does the
// errors.As + AwaitInput in one call.
type PauseError struct {
	Request core.Awaitable
}

func (e *PauseError) Error() string {
	return fmt.Sprintf("hitl.PauseError: tool requested pause (awaitable %q)", e.Request.ID())
}

// HandlePause inspects err for a *PauseError. When found, it parks
// the carried Awaitable on the process via pc.AwaitInput and returns
// (ActionWaiting, true) — the action body should return that status
// immediately without further error handling.
//
// When err carries no PauseError, returns (_, false); the returned
// status is undefined and the caller is still responsible for
// handling err.
//
//	text, _, err := req.Call().Text(ctx)
//	if status, paused := hitl.HandlePause(pc, err); paused {
//	    return status
//	}
//	if err != nil {
//	    return core.ActionFailed
//	}
func HandlePause(pc *core.ProcessContext, err error) (core.ActionStatus, bool) {
	pe, ok := errors.AsType[*PauseError](err)
	if !ok {
		return 0, false
	}
	return pc.AwaitInput(pe.Request), true
}

// AwaitDecider chooses whether to pause a tool call. It receives the
// ctx the tool was invoked with (which carries the running process
// via [core.WithProcess] — use [core.ProcessFrom] to reach the
// blackboard from inside a decider) and the raw JSON argument blob
// the LLM produced.
//
// Return a non-nil [core.Awaitable] to suspend the process; return
// nil to let the underlying tool run normally. The decider is called
// on every tool invocation — caller is responsible for tracking
// "already asked, don't ask again" state, typically via the
// blackboard.
type AwaitDecider func(ctx context.Context, arguments string) core.Awaitable

// RequireAwait wraps tool with an [AwaitDecider]. Whenever the decider
// returns a non-nil Awaitable, the wrapped Call returns a *PauseError
// carrying it; otherwise the underlying tool runs normally. Mirrors
// embabel's `Tool.withAwaiting(decider)`. Returns an error when tool
// or decider is nil — caller decides whether to surface or panic.
func RequireAwait(tool chat.Tool, decider AwaitDecider) (chat.Tool, error) {
	if tool == nil {
		return nil, errors.New("hitl.RequireAwait: tool must not be nil")
	}
	if decider == nil {
		return nil, errors.New("hitl.RequireAwait: decider must not be nil")
	}
	return &awaitingTool{delegate: tool, decider: decider}, nil
}

type awaitingTool struct {
	delegate chat.Tool
	decider  AwaitDecider
}

func (t *awaitingTool) Definition() chat.ToolDefinition { return t.delegate.Definition() }
func (t *awaitingTool) Metadata() chat.ToolMetadata     { return t.delegate.Metadata() }

func (t *awaitingTool) Call(ctx context.Context, arguments string) (string, error) {
	if a := t.decider(ctx, arguments); a != nil {
		return "", &PauseError{Request: a}
	}
	return t.delegate.Call(ctx, arguments)
}

// ConfirmationPrompter renders a confirmation message from the raw
// JSON arguments the LLM constructed for the tool call.
type ConfirmationPrompter func(arguments string) string

// RequireConfirmation wraps tool to demand a yes/no from the user
// before each invocation. prompter renders the confirmation message
// from the call arguments; onResponse receives the user's bool reply
// and returns the resulting [core.ResponseImpact] (typically
// [core.ImpactUpdated] when the handler writes the decision
// to the blackboard so the next tick observes it).
//
// onResponse may be nil when the action body itself stages state via
// other means; the awaitable still fires but the impact is treated
// as Unchanged.
//
// Mirrors embabel's `Tool.withConfirmation { msg }`.
func RequireConfirmation(
	tool chat.Tool,
	prompter ConfirmationPrompter,
	onResponse func(approved bool) core.ResponseImpact,
) (chat.Tool, error) {
	if tool == nil {
		return nil, errors.New("hitl.RequireConfirmation: tool must not be nil")
	}
	if prompter == nil {
		return nil, errors.New("hitl.RequireConfirmation: prompter must not be nil")
	}
	handler := onResponse
	if handler == nil {
		handler = func(bool) core.ResponseImpact { return core.ImpactUnchanged }
	}
	return RequireAwait(tool, func(_ context.Context, arguments string) core.Awaitable {
		return NewConfirmation(prompter(arguments), handler)
	})
}

// RequireType wraps tool to demand a typed value of T from the user
// before each invocation. prompter composes a request message from
// the raw call arguments; onResponse receives the user-supplied
// value and returns its [core.ResponseImpact].
//
// onResponse may be nil when the action body itself binds state.
//
// Mirrors embabel's `Tool.requireType<T>(message)`.
func RequireType[T any](
	tool chat.Tool,
	prompter ConfirmationPrompter,
	onResponse func(value T) core.ResponseImpact,
) (chat.Tool, error) {
	if tool == nil {
		return nil, errors.New("hitl.RequireType: tool must not be nil")
	}
	if prompter == nil {
		return nil, errors.New("hitl.RequireType: prompter must not be nil")
	}
	handler := onResponse
	if handler == nil {
		handler = func(T) core.ResponseImpact { return core.ImpactUnchanged }
	}
	return RequireAwait(tool, func(_ context.Context, arguments string) core.Awaitable {
		return NewTypedRequest[string, T](prompter(arguments), handler)
	})
}
