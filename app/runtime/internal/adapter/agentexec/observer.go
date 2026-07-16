package agentexec

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
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
// Implementations must be safe for concurrent calls because separate turns may
// share one observer backend, even though one target tool-loop executes calls
// serially.
type toolObserver interface {
	// ApproveToolCall is the gate consulted BEFORE every tool call.
	// It returns a verdict telling the decorator whether the call runs,
	// is denied (short-circuited to a recoverable result), or must pause
	// the process for user approval (HITL R model, API.md §6): a non-nil
	// Verdict.Interrupt makes the call return the framework's durable
	// Suspension error, parking the process at [core.StatusWaiting]. The
	// client answers via a continuation run.
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

	// OnUsage is invoked once per completed LLM round (right after the
	// round's tokens are recorded into the process budget), carrying the
	// turn's cumulative token roll-up and cost so far. This is the mid-run
	// usage signal — a live "tokens / cost spent" readout — distinct from the
	// final per-turn total that lands on TurnEnd. costUSD is zero when no
	// pricing hook is configured (the wire layer omits it rather than showing
	// a fabricated $0).
	//
	// contextTokens is THIS round's prompt-token count (not cumulative) — the
	// size of the context the model was just sent, i.e. how full the window is
	// right now. It grows across rounds/turns as history accumulates and drops
	// after a compaction, so the client can render a live context-occupancy
	// gauge (distinct from the summed usage, which only ever grows).
	OnUsage(usage accounting.TokenUsage, costUSD float64, contextTokens int64)
}

var toolObserverKey = core.MustDependencyKey[toolObserver]("lyra.tool_observer")

// ToolApprovalVerdict is the decorator's instruction for one gated tool
// call (API.md §6 HITL). Exactly one outcome applies:
//
//   - Interrupt != nil → suspend the run for human input (R model); the
//     chat tool loop propagates the durable Suspension so the action parks. On resume
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

// observerFrom extracts the [toolObserver] the engine attached to the typed
// process dependency scope via [Engine.StartTurn]. Returns nil when no observer is
// registered — Action bodies treat that as "no streaming hook
// wired" and skip the per-chunk callback.
//
// Lives here because the key and dependency type are Lyra-specific.
func observerFrom(dependencies *core.Dependencies) toolObserver {
	if dependencies == nil {
		return nil
	}
	observer, _ := core.LookupDependency(dependencies, toolObserverKey)
	return observer
}

// toolObserverMiddleware is the process-scope [core.ToolMiddleware]
// the engine attaches when [StartTurn] is called with an observer.
// It wraps every resolved [core.AgentTool] with [observedTool] so
// invocations land on the observer without changing the underlying
// tool implementation.
type toolObserverMiddleware struct {
	observer toolObserver
}

func (d *toolObserverMiddleware) ToolObserver() toolObserver { return d.observer }

// Name implements [core.Extension]. The constant string is fine —
// process-scope extensions allow name collisions with engine
// scope, and this decorator is process-scoped.
func (d *toolObserverMiddleware) Name() string { return "tool-observer" }

// WrapTool wraps tool with [observedTool], threading the
// observer into every Call so start / end notifications fire.
// Action is intentionally ignored — Lyra emits per-tool, not
// per-action, events.
func (d *toolObserverMiddleware) WrapTool(_ core.ProcessView, _ core.Action, tool tools.Tool) tools.Tool {
	return &observedTool{inner: tool, observer: d.observer}
}

// observedTool is the per-call wrapper. CallID is generated fresh
// per invocation so two concurrent calls to the same tool stay
// distinguishable on the observer side.
type observedTool struct {
	inner    tools.Tool
	observer toolObserver
}

func (o *observedTool) Definition() chat.ToolDefinition { return o.inner.Definition() }

// ReturnsDirect forwards the wrapped tool's return-direct declaration. This
// decorator wraps every resolved tool, so dropping the marker would turn
// return-direct tools into regular continuation tools.
func (o *observedTool) ReturnsDirect() bool {
	if direct, ok := o.inner.(interface{ ReturnsDirect() bool }); ok {
		return direct.ReturnsDirect()
	}
	return false
}

func (o *observedTool) Call(ctx context.Context, arguments string) (string, error) {
	callID := uuid.NewString()
	name := o.inner.Definition().Name

	v := o.observer.ApproveToolCall(ctx, callID, name, arguments)
	switch {
	case v.Interrupt != nil:
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
