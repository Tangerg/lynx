// Package chat defines the ChatService — Lyra's one-turn dispatch
// surface. A turn is the unit of interaction: client sends one
// message, runtime drives one (possibly multi-tool) round, runtime
// streams events back, turn ends with a [TurnEnd] event.
package chat

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// StartTurnRequest is the input to [Service.StartTurn]. SessionID
// binds the turn to its conversation; Message is the user's input;
// PlanMode opts into the [PlanGenerated] preview flow (M6).
type StartTurnRequest struct {
	SessionID string
	Message   string

	// PlanMode requests a plan preview before execution. When true
	// the runtime emits a [PlanGenerated] event and pauses until
	// the client either continues the turn (via a separate call,
	// TBD) or cancels. M6 milestone.
	PlanMode bool

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. On
	// overrun the turn stops cleanly after the current round and ends
	// with Reason=[TurnEndBudgetExceeded], the partial reply already
	// streamed. In-process / automated callers set this; it is not
	// (yet) carried on the wire.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost the same way MaxBudget caps
	// tokens (0 = no cap). Needs a configured pricing hook; same
	// TurnEndBudgetExceeded stop. Also not (yet) on the wire.
	MaxCostUSD float64
}

// TurnHandle uniquely identifies an in-flight turn. Returned by
// [Service.StartTurn] and used to address subsequent operations
// (steering injection, cancellation).
type TurnHandle struct {
	SessionID string
	TurnID    string
}

// Service is the ChatService contract.
//
// A typical interaction:
//
//	handle, err := chat.StartTurn(ctx, req)
//	events := chat.Events(ctx, handle)
//	for ev := range events {
//	    switch e := ev.(type) {
//	    case MessageDelta: ui.AppendText(e.Text)
//	    case ToolCallStart: ui.ShowSpinner(e.ToolName)
//	    case PlanGenerated: ui.ShowPlan(e.Plan)  // plan-mode only
//	    case TurnEnd: return
//	    case Error: handleErr(e)
//	    }
//	}
//
// In plan mode the turn pauses after [PlanGenerated]; call
// [ContinuePlan] with the user's decision to resume.
//
// A turn outlives the ctx that started it: StartTurn derives the turn's
// own context from a background root so the caller's ctx ending (e.g. the
// StartTurn RPC returning) does not kill the in-flight turn. To stop a
// turn, call [Service.Cancel] — closing the ctx you passed in has no
// effect on a running turn.
type Service interface {
	// StartTurn launches a new turn against the given session. Returns
	// a handle the caller uses to subscribe to events. The method
	// returns as soon as the turn is scheduled — actual LLM work
	// happens asynchronously and surfaces via [Events].
	StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error)

	// Events returns a pull iterator over a turn's events: range it to
	// drain the stream, which ends when the turn does (success or error).
	// It is single-consumer — one drain loop per turn. ctx bounds how
	// long the caller listens: when ctx is done the iterator stops
	// yielding, but the turn keeps running on its own lifetime (use
	// [Service.Cancel] to stop the turn itself). Returns [ErrTurnNotFound]
	// once the turn has ended.
	Events(ctx context.Context, handle TurnHandle) (iter.Seq[Event], error)

	// InjectSteering delivers a user message mid-turn. The runtime
	// queues it until the next tool boundary then injects it into the
	// model's context. No-op when the turn has already completed.
	// (M-future; signature reserved to stabilize the surface.)
	InjectSteering(ctx context.Context, handle TurnHandle, message string) error

	// ContinuePlan resumes a plan-mode turn that paused after emitting
	// a [PlanGenerated] event. Decision = Approve runs the plan
	// through the regular tool-loop path; Reject ends the turn with
	// Reason=Canceled. Returns [ErrTurnNotFound] when the turn is
	// not in the plan-pending state.
	ContinuePlan(ctx context.Context, handle TurnHandle, decision PlanDecision) error

	// Cancel stops the turn immediately, drains pending tool calls
	// safely, and emits a final [TurnEnd] event with Reason=Canceled.
	Cancel(ctx context.Context, handle TurnHandle) error
}

// PlanDecision is the client's response to a paused plan-mode turn.
type PlanDecision int

const (
	// PlanApprove tells the runtime to execute the proposed plan.
	PlanApprove PlanDecision = iota
	// PlanReject aborts the turn — Lyra emits TurnEnd(Canceled)
	// without running any tools.
	PlanReject
)

func (d PlanDecision) String() string {
	switch d {
	case PlanApprove:
		return "approve"
	case PlanReject:
		return "reject"
	default:
		return "unknown"
	}
}

// ------------------------------------------------------------------
// Event types
// ------------------------------------------------------------------

// Event is the sealed sum type emitted on a turn's event channel.
// Concrete event types implement this marker so callers can type-switch.
//
// All events carry [BaseEvent] for routing — SessionID + TurnID + Seq
// + Timestamp. Seq is monotone within a turn so clients can detect
// gaps after reconnects (Phase 2).
//
// Concrete event types implement [stamp] so the runtime replaces
// the full BaseEvent header in one call. Call sites construct
// events with their type-specific fields only — the dispatcher
// (emit, in inmemory.go) fills the four routing fields uniformly.
// Adding a new event = adding the struct + one stamp method, nothing else.
type Event interface {
	isLyraEvent()
	stamp(b BaseEvent) Event
}

// BaseEvent is the common header on every event. Embedded by all
// concrete event types so the type switch sees them as Events but
// also gives uniform access to routing metadata.
type BaseEvent struct {
	SessionID string
	TurnID    string
	Seq       uint64
	Timestamp time.Time
}

func (BaseEvent) isLyraEvent() {}

// TurnStart fires once at the very beginning of a turn. Carries the
// resolved model name and any system-prompt summary the client wants
// to display.
type TurnStart struct {
	BaseEvent
	Model string
}

// MessageDelta is one streaming chunk of assistant text. Clients
// concatenate Text values in arrival order.
type MessageDelta struct {
	BaseEvent
	Text string
}

// ReasoningDelta is one streaming chunk of extended-thinking
// (reasoning) text — Claude's <thinking> blocks, OpenAI o-series
// reasoning summaries, DeepSeek-R1 chain-of-thought. Distinct
// from [MessageDelta] so transports can render reasoning
// separately (collapsed by default, dimmed, behind a toggle).
type ReasoningDelta struct {
	BaseEvent
	Text string
}

// ToolCallStart fires when the model invokes a tool. Arguments is
// the raw JSON the model emitted.
//
// Reserved for M2 (tool 集); declared in M1 so the event sum type is
// stable from the start.
type ToolCallStart struct {
	BaseEvent
	CallID    string
	ToolName  string
	Arguments string
}

// PlanGenerated fires once during a plan-mode turn, after the LLM
// produces the step-list but before any tool calls run. The turn
// is paused at this point — the client inspects the plan, then
// calls [Service.ContinuePlan] with an Approve / Reject decision.
//
// Plan is the LLM's raw markdown — typically a numbered list. The
// runtime makes no attempt to parse it into structured steps;
// downstream consumers render the markdown verbatim.
type PlanGenerated struct {
	BaseEvent
	Plan string
}

// ToolCallApproval fires when a destructive tool wants to run and
// the configured approval mode requires user consent. The turn
// blocks until the client resolves the matching [approval.Request]
// via [approval.Service.Decide]; on Deny the engine surfaces the
// refusal back to the model so it can recover or abandon the
// approach.
//
// Emitted only when the runtime is wired with a non-yolo
// [approval.Service]; transports gate on this event to render
// the consent prompt.
type ToolCallApproval struct {
	BaseEvent
	Request approval.Request
}

// ToolCallEnd fires when the tool finishes. Output is the tool's
// returned text; Err is non-empty when the tool failed.
//
// Reserved for M2.
type ToolCallEnd struct {
	BaseEvent
	CallID string
	Output string
	Err    string
}

// CompactBoundary fires when the runtime auto-compacts the
// conversation after a turn — the older messages were folded into a
// single summary so context stays within bounds. Surfacing it lets
// clients show "context compacted (120 → 40 messages)" instead of
// silently dropping history. Mirrors the SDK's
// SDKCompactBoundaryMessage. Fires before [TurnEnd].
type CompactBoundary struct {
	BaseEvent
	MessagesBefore int
	MessagesAfter  int
}

// MemoryUpdated fires when the runtime mines durable facts out of the
// finished turn and appends them to project memory (LYRA.md). Facts
// is the markdown the runtime saved; clients can surface "saved notes
// to memory". Only fires when extraction actually wrote something,
// and always after a [CompactBoundary] (extraction is gated on
// compaction). Mirrors the spirit of the SDK's memory events.
type MemoryUpdated struct {
	BaseEvent
	Facts string
}

// TurnEnd fires once at the end of a turn. Reason explains why the
// turn ended; TokenUsage / CostUSD are the rolled-up totals for the
// turn (sum across every LLM call inside it).
type TurnEnd struct {
	BaseEvent
	Reason       TurnEndReason
	TokenUsage   TokenUsage
	UsageByModel []ModelUsage // per-model breakdown; one entry for a single-model turn
	CostUSD      float64      // turn cost; zero unless a pricing hook is configured (engine.Config.Pricing)
	Duration     time.Duration
}

// ErrorEvent fires when an unrecoverable error aborts the turn. The
// turn channel still closes after this event so receivers don't need
// to special-case the end.
type ErrorEvent struct {
	BaseEvent
	Message string
	Code    string // stable error code; see errors.go
}

// stamp implementations — concrete events return themselves with
// the BaseEvent header replaced wholesale. Value-typed events are
// the right idiom here: the dispatcher (impl.emit) takes ownership
// of a copy so concurrent receivers can't observe a half-stamped
// header. Each method is one assignment because the new BaseEvent
// already carries every routing field — emit builds it once per
// call.

func (e TurnStart) stamp(b BaseEvent) Event        { e.BaseEvent = b; return e }
func (e MessageDelta) stamp(b BaseEvent) Event     { e.BaseEvent = b; return e }
func (e ReasoningDelta) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e ToolCallStart) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }
func (e ToolCallEnd) stamp(b BaseEvent) Event      { e.BaseEvent = b; return e }
func (e ToolCallApproval) stamp(b BaseEvent) Event { e.BaseEvent = b; return e }
func (e PlanGenerated) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }
func (e CompactBoundary) stamp(b BaseEvent) Event  { e.BaseEvent = b; return e }
func (e MemoryUpdated) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }
func (e TurnEnd) stamp(b BaseEvent) Event          { e.BaseEvent = b; return e }
func (e ErrorEvent) stamp(b BaseEvent) Event       { e.BaseEvent = b; return e }

// TurnEndReason enumerates why a turn ended.
type TurnEndReason int

const (
	// TurnEndCompleted — the model returned a stop-marker normally.
	TurnEndCompleted TurnEndReason = iota
	// TurnEndCancelled — the client called [Service.Cancel] or ctx was
	// canceled.
	TurnEndCancelled
	// TurnEndErrored — the turn aborted on error. An [ErrorEvent]
	// fires before [TurnEnd] in this case.
	TurnEndErrored
	// TurnEndBudgetExceeded — the turn hit [StartTurnRequest.MaxBudget]
	// and stopped cleanly after the current round. Not an error: the
	// partial reply already streamed; TokenUsage reflects what was
	// spent.
	TurnEndBudgetExceeded
)

func (r TurnEndReason) String() string {
	switch r {
	case TurnEndCompleted:
		return "completed"
	case TurnEndCancelled:
		return "canceled"
	case TurnEndErrored:
		return "errored"
	case TurnEndBudgetExceeded:
		return "budget_exceeded"
	default:
		return "unknown"
	}
}

// TokenUsage is the per-turn token roll-up. Alias for
// [engine.TokenUsage] — the engine owns the canonical shape and
// the chat package re-exports it so transport adapters can stay
// scoped to one import.
type TokenUsage = engine.TokenUsage

// ModelUsage is the per-model token breakdown re-exported from the
// engine, so transport adapters consuming [TurnEnd.UsageByModel] stay
// scoped to one import.
type ModelUsage = engine.ModelUsage
