package engine

import (
	"context"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
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
	// ApproveToolCall is the gate consulted BEFORE every tool call.
	// It returns a verdict telling the decorator whether the call runs,
	// is denied (short-circuited to a recoverable result), or must pause
	// the process for user approval (HITL R model, API.md §6): a non-nil
	// Verdict.Pause makes the call return a [hitl.PauseError], which
	// suspends the run at [core.StatusWaiting]; the client answers via a
	// continuation run.
	//
	// The decider MUST be non-blocking — it records pending / decided
	// state out of band (typically the process blackboard, keyed by the
	// stable tool name + arguments so the verdict survives the LLM round
	// re-running on resume) rather than waiting on a channel.
	//
	// Receives the same callID it will later get on Start / End so the
	// implementation can pair the gate with the lifecycle.
	ApproveToolCall(ctx context.Context, callID, toolName, arguments string) ToolApprovalVerdict

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

// ToolApprovalVerdict is the decorator's instruction for one gated tool
// call (API.md §6 HITL). Exactly one outcome applies:
//
//   - Pause != nil → suspend the run for user approval (R model); the
//     call returns a [hitl.PauseError] carrying this awaitable.
//   - Denied       → short-circuit with DenyReason as a recoverable
//     tool result (the model adapts), no execution.
//   - zero value   → run the tool normally.
type ToolApprovalVerdict struct {
	Pause      core.Awaitable
	Denied     bool
	DenyReason string
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

	switch v := o.observer.ApproveToolCall(ctx, callID, name, arguments); {
	case v.Pause != nil:
		// Suspend the run for approval (HITL R). The PauseError bubbles
		// through the chat tool middleware up to the action body, which
		// parks the process on AwaitInput (StatusWaiting). No Start/End
		// fires — the call hasn't begun; on resume the LLM round re-runs
		// and the decider, seeing the recorded verdict, runs or denies.
		return "", &hitl.PauseError{Request: v.Pause}
	case v.Denied:
		// Recoverable denial: the model sees DenyReason as the tool
		// result and adapts instead of aborting. Start/End still fire so
		// UI counts stay matched.
		o.observer.OnToolCallStart(callID, name, arguments)
		o.observer.OnToolCallEnd(callID, name, v.DenyReason, nil)
		return v.DenyReason, nil
	}

	o.observer.OnToolCallStart(callID, name, arguments)
	output, err := o.inner.Call(ctx, arguments)
	o.observer.OnToolCallEnd(callID, name, output, err)

	return output, err
}
