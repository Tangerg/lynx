package kernel

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// ErrToolDenied is the sentinel the gate hands the observer's OnToolCallEnd
// when a tool call is denied by the approval verdict (vs. failing during
// execution). Lets the wire layer render a "denied" terminal distinct from a
// generic tool failure (and from a green success). errors.Is-matchable.
var ErrToolDenied = errors.New("engine.ErrToolDenied: tool call denied by user")

// toolObserver receives both tool-call lifecycle notifications and
// streaming assistant text deltas as a turn unfolds. Each tool call
// fires one OnToolCallStart followed by one OnToolCallEnd carrying
// the same opaque CallID; the assistant text arrives in zero or
// more OnMessageDelta calls between (and around) tool calls.
//
// Implementations must be safe for concurrent calls — a chat turn
// may dispatch multiple tools simultaneously when the model emits
// parallel tool_calls.
type toolObserver interface {
	// ApproveToolCall is the gate consulted BEFORE every tool call.
	// It returns a verdict telling the decorator whether the call runs,
	// is denied (short-circuited to a recoverable result), or must pause
	// the process for user approval (HITL R model, API.md §6): a non-nil
	// Verdict.Interrupt makes the call return that error (a
	// [hitl.InterruptError], which satisfies [chat.ToolHalt]), suspending the
	// run at [core.StatusWaiting]; the client answers via a continuation run.
	//
	// The decider MUST be non-blocking — it records pending / decided
	// state out of band (typically the process blackboard, keyed by the
	// stable tool name + arguments so the verdict matches the same parked
	// tool call when it is re-presented on resume) rather than waiting on a
	// channel.
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
}

// ToolApprovalVerdict is the decorator's instruction for one gated tool
// call (API.md §6 HITL). Exactly one outcome applies:
//
//   - Interrupt != nil → suspend the run for human input (R model); the
//     call returns this error (an agent/hitl.InterruptError), which the
//     chat tool loop exits on and propagates so the action parks. On resume
//     the gate is consulted again and returns one of the outcomes below.
//   - Denied           → short-circuit with DenyReason as a recoverable
//     tool result (the model adapts), no execution.
//   - zero value       → run the tool. Arguments, when non-empty, overrides
//     the call's arguments (the "approve with edits" affordance).
type ToolApprovalVerdict struct {
	Interrupt  error
	Denied     bool
	DenyReason string
	Arguments  string
}

// observerFrom extracts the [toolObserver] the engine attached to
// opts via [Engine.RunChat]. Returns nil when no observer is
// registered — Action bodies treat that as "no streaming hook
// wired" and skip the per-chunk callback.
//
// Lives here (not on ProcessContext) because action bodies are the
// only callers and the lookup is type-specific to Lyra's decorator.
func observerFrom(opts *core.ProcessOptions) toolObserver {
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
	observer toolObserver
}

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with platform
// scope, and this decorator is process-scoped.
func (d *toolObserverDecorator) Name() string { return "tool-observer" }

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
	observer toolObserver
}

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }
func (o *observedTool) Metadata() chat.ToolMetadata     { return o.inner.Metadata() }

// ConcurrencyKey forwards the wrapped tool's concurrency declaration (the tool
// loop's optional ConcurrentTool contract), matched structurally so the kernel
// needn't import the loop driver. This MUST be forwarded: this decorator wraps
// EVERY tool the agent resolves, so dropping the method would strip every
// tool's declaration and force the loop to run all calls exclusively (serial),
// silently defeating parallel tool execution — e.g. concurrent `task`
// sub-agents or distinct-file edits. A wrapped tool that declares nothing stays
// exclusive (concurrent=false).
func (o *observedTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if c, ok := o.inner.(interface {
		ConcurrencyKey(string) (string, bool)
	}); ok {
		return c.ConcurrencyKey(arguments)
	}
	return "", false
}

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	callID := uuid.NewString()
	name := o.inner.Definition().Name

	v := o.observer.ApproveToolCall(ctx, callID, name, arguments)
	switch {
	case v.Interrupt != nil:
		// Interrupt the run for human input (HITL R). The InterruptError
		// bubbles through the chat tool loop (which exits immediately and
		// carries the resumable conversation) up to the action body, which
		// parks the process on AwaitInput (StatusWaiting). No Start/End
		// fires — the call hasn't begun; on resume the gate is consulted
		// again, sees the recorded resolution, and runs or denies.
		return "", v.Interrupt
	case v.Denied:
		// Recoverable denial: the model sees DenyReason as the tool
		// result and adapts instead of aborting. Start/End still fire so
		// UI counts stay matched; End carries ErrToolDenied so the wire
		// renders a distinct "denied" terminal (not a green success).
		o.observer.OnToolCallStart(callID, name, arguments)
		o.observer.OnToolCallEnd(callID, name, v.DenyReason, ErrToolDenied)
		return v.DenyReason, nil
	}

	// Approved. v.Arguments overrides the call's arguments when the human
	// edited them before approving (the "approve with edits" affordance).
	if v.Arguments != "" {
		arguments = v.Arguments
	}
	o.observer.OnToolCallStart(callID, name, arguments)
	output, err := o.inner.Call(ctx, arguments)
	o.observer.OnToolCallEnd(callID, name, output, err)

	return output, err
}
