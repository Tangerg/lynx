package engine

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// ToolObserver receives both tool-call lifecycle notifications and
// streaming assistant text deltas as a turn unfolds. Each tool call
// fires one OnToolCallStart followed by one OnToolCallEnd carrying
// the same opaque CallID; the assistant text arrives in zero or
// more OnMessageDelta calls between (and around) tool calls.
//
// Implementations must be safe for concurrent calls — a chat turn
// may dispatch multiple tools simultaneously when the model emits
// parallel tool_calls.
type ToolObserver interface {
	// OnToolCallApprove is the gate fired BEFORE every tool call —
	// implementations decide whether the call may proceed. Returning
	// a non-nil error short-circuits the tool: the engine reports
	// the error back to the model so it can recover. nil means
	// "go ahead". Implementations that don't gate should return nil
	// unconditionally (the engine's default observer behavior).
	//
	// Receives the same callID it will later get on Start / End so
	// the implementation can pair the gate with the lifecycle.
	OnToolCallApprove(ctx context.Context, callID, toolName, arguments string) error

	OnToolCallStart(callID, toolName, arguments string)
	OnToolCallEnd(callID, toolName, output string, err error)

	// OnMessageDelta is invoked for every non-empty text chunk the
	// model streams out. Implementations typically append the chunk
	// to a UI buffer or forward it to an event channel.
	OnMessageDelta(text string)

	// OnReasoningDelta is invoked for every non-empty reasoning
	// (extended thinking) chunk the model streams out — distinct
	// from final-text chunks so UIs can render thinking separately
	// (e.g. dimmed, collapsed, or behind a "show reasoning" toggle).
	OnReasoningDelta(text string)

	// OnPlanGenerated fires once in plan mode when the agent has
	// drafted a plan and is about to park on approval (AwaitInput).
	// Implementations surface it so the client can render the plan and
	// later resume the turn via the approval path.
	OnPlanGenerated(plan string)
}

// ObserverFrom extracts the [ToolObserver] the engine attached to
// opts via [Engine.RunChat]. Returns nil when no observer is
// registered — Action bodies treat that as "no streaming hook
// wired" and skip the per-chunk callback.
//
// Lives here (not on ProcessContext) because action bodies are the
// only callers and the lookup is type-specific to Lyra's decorator.
func ObserverFrom(opts *core.ProcessOptions) ToolObserver {
	if opts == nil {
		return nil
	}
	for _, ext := range opts.Extensions {
		if d, ok := ext.(*toolObserverDecorator); ok {
			return d.observer
		}
	}
	return nil
}

// toolObserverDecorator is the process-scope [core.ToolDecorator]
// the engine attaches when [RunChat] is called with an observer.
// It wraps every resolved [core.AgentTool] with [observedTool] so
// invocations land on the observer without changing the underlying
// tool implementation.
type toolObserverDecorator struct {
	observer ToolObserver
}

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with platform
// scope, and this decorator is process-scoped.
func (d *toolObserverDecorator) Name() string { return "lyra-tool-observer" }

// DecorateTool wraps tool with [observedTool], threading the
// observer into every Call so start / end notifications fire.
// Action is intentionally ignored — Lyra emits per-tool, not
// per-action, events.
func (d *toolObserverDecorator) DecorateTool(_ core.Process, _ core.Action, tool core.AgentTool) core.AgentTool {
	return &observedTool{inner: tool, observer: d.observer}
}

// observedTool is the per-call wrapper. CallID is generated fresh
// per invocation so two concurrent calls to the same tool stay
// distinguishable on the observer side.
type observedTool struct {
	inner    chat.Tool
	observer ToolObserver
}

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }
func (o *observedTool) Metadata() chat.ToolMetadata     { return o.inner.Metadata() }

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	callID := uuid.NewString()
	name := o.inner.Definition().Name

	if err := o.observer.OnToolCallApprove(ctx, callID, name, arguments); err != nil {
		// Surface the gate refusal as a recoverable tool *result*
		// (text the model can read), not a Go error. Returning an
		// error would propagate up the chat tool middleware and
		// abort the turn — denying the model a chance to back off
		// and try a different path. Observers still see a
		// (start, end) pair so UI counts stay matched.
		denial := "tool execution denied by user: " + err.Error()
		o.observer.OnToolCallStart(callID, name, arguments)
		o.observer.OnToolCallEnd(callID, name, denial, nil)
		return denial, nil
	}

	o.observer.OnToolCallStart(callID, name, arguments)
	output, err := o.inner.Call(ctx, arguments)
	o.observer.OnToolCallEnd(callID, name, output, err)

	return output, err
}
