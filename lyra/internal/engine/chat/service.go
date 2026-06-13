// Package chat defines the ChatService — Lyra's one-turn dispatch
// surface. A turn is the unit of interaction: client sends one
// message, runtime drives one (possibly multi-tool) round, runtime
// streams events back, turn ends with a [TurnEnd] event.
package chat

import (
	"context"
	"iter"
	"time"

	corechat "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
)

// clientResolver resolves a per-turn chat client for an explicit
// (provider, model) — the seam the multi-provider runtime plugs in so a turn
// can run against a model other than the default. Unexported: chat's own
// consumer-side abstraction, satisfied implicitly by the runtime's provider
// registry (nothing outside this package names it). Returns an error when the
// provider isn't configured / enabled.
type clientResolver interface {
	ResolveClient(ctx context.Context, provider, model string) (*corechat.Client, error)
}

// StartTurnRequest is the input to [Service.StartTurn]. SessionID
// binds the turn to its conversation; Message is the user's input;
// PlanMode opts into the [PlanGenerated] preview flow (M6).
type StartTurnRequest struct {
	SessionID string
	Message   string

	// Cwd is the session's working directory — the project root the turn's
	// filesystem + bash tools run in. Resolved from Session.cwd by the
	// caller (runs.start). Empty falls back to the engine's default workdir.
	Cwd string

	// Provider + Model select the model this turn runs against (the wire
	// runs.start{providerId, model}). Both empty uses the runtime's default;
	// both set resolves that provider+model client via the clientResolver and
	// runs the turn against it. The provider is explicit — never inferred.
	Provider string
	Model    string

	// PlanMode requests a plan preview before execution. When true
	// the runtime emits a [PlanGenerated] event and pauses until
	// the client either continues the turn (via a separate call,
	// TBD) or cancels. M6 milestone.
	PlanMode bool

	// ChatMode runs the turn tool-less (runs.start mode=chat): a plain
	// single-round LLM exchange with no filesystem / bash / delegation tools.
	ChatMode bool

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

// RehydrateRequest carries the inputs to rebuild a turn from a persisted
// process snapshot and resume it after a restart. ProcessID is the
// agent-process snapshot key (recorded on the open interrupt); SessionID
// rebinds chat-memory; Approved is the human decision delivered to the
// re-parked process.
type RehydrateRequest struct {
	SessionID string
	ProcessID string
	Approved  bool
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
// In plan mode the turn pauses after [PlanGenerated]; call [Resume]
// with the user's decision — approved=true continues, false rejects.
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
	// queues it in the active turn state and flushes it into the
	// conversation history once the turn completes, so the next turn
	// sees the steering. No-op when the turn has already completed.
	InjectSteering(ctx context.Context, handle TurnHandle, message string) error

	// Resume answers a turn parked on a HITL interrupt (a gated tool
	// call awaiting approval, a plan awaiting review, or an ask_user
	// question — all surface as a [TurnInterrupted] event). The structured
	// [interrupts.Resolution] carries the decision (approve/deny, with
	// optionally edited tool arguments) or the question's answer. The
	// continuation streams onto the SAME turn's event channel — call
	// [Events] again after Resume to drain it. Returns [ErrTurnNotFound]
	// when the turn isn't parked.
	Resume(ctx context.Context, handle TurnHandle, resolution interrupts.Resolution) error

	// ProcessID returns the agent-process id backing a live (parked) turn
	// — the snapshot key the runtime records so a restart can rebuild the
	// process via [Rehydrate]. Returns [ErrTurnNotFound] when the turn
	// isn't live, or an error when it hasn't dispatched a process yet.
	ProcessID(ctx context.Context, handle TurnHandle) (string, error)

	// Rehydrate rebuilds a turn whose live in-memory state was lost (the
	// backend restarted) from the persisted process snapshot identified by
	// req.ProcessID, then resumes it with req.Approved. It registers a
	// fresh turn and returns its handle; the continuation streams on that
	// handle's channel (subscribe via [Events]). This is the cross-restart
	// counterpart to [Resume], used when [Resume] would return
	// [ErrTurnNotFound]. Errors when the snapshot is missing / unrestorable.
	Rehydrate(ctx context.Context, req RehydrateRequest) (TurnHandle, error)

	// Cancel stops the turn immediately, drains pending tool calls
	// safely, and emits a final [TurnEnd] event with Reason=Canceled.
	Cancel(ctx context.Context, handle TurnHandle) error

	// SetInterruptKinds records the HITL interrupt kinds the connected
	// client can answer (negotiated at runtime.initialize via
	// ClientCapabilities.InterruptKinds). A turn about to park on a kind
	// not in this set is auto-denied instead of left unanswerable
	// (API.md §6.2 anti-deadlock). An empty set gates every kind; never
	// calling it leaves the permissive default (surface all kinds).
	SetInterruptKinds(kinds []string)
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
type ToolCallStart struct {
	BaseEvent
	CallID    string
	ToolName  string
	Arguments string
}

// PlanGenerated fires once during a plan-mode turn, after the LLM
// produces the step-list but before any tool calls run. The turn
// is paused at this point — the client inspects the plan, then
// calls [Service.Resume] with approve (true) / reject (false).
//
// Plan is the LLM's raw markdown — typically a numbered list. The
// runtime makes no attempt to parse it into structured steps;
// downstream consumers render the markdown verbatim.
type PlanGenerated struct {
	BaseEvent
	Plan string
}

// ToolCallEnd fires when the tool finishes. Output is the tool's
// returned text; Err is non-empty when the tool failed.
type ToolCallEnd struct {
	BaseEvent
	CallID string
	Output string
	Err    string
	// Denied is true when the call ended because the approval verdict
	// denied it (not an execution failure). The wire layer renders a
	// distinct "denied" terminal — see [engine.ErrToolDenied].
	Denied bool
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

// TurnInterrupted fires when the turn parks for human input (HITL R
// model): a gated tool call needs approval, or a plan needs review. The
// turn does NOT end — it suspends at [core.StatusWaiting]; the client
// answers via [Service.Resume], which continues the same turn (its
// events resume on the same channel). Carries the pending interrupt(s).
type TurnInterrupted struct {
	BaseEvent
	Interrupts []Interrupt
}

// Interrupt is one pending HITL request surfaced by [TurnInterrupted].
// Kind is the wire interrupt kind (API.md §6: "approval" | "question" |
// "toolResult") so it lines up with what the client negotiates in
// ClientCapabilities.InterruptKinds. Payload is the awaitable's prompt —
// an [ApprovalPrompt] for a gated tool call ("approval"), or the plan
// markdown string for a plan awaiting review ("question").
type Interrupt struct {
	Kind    string // "approval" | "question"
	Payload any
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
// the right idiom here: the dispatcher (emit, in inmemory.go) takes
// ownership of a copy so concurrent receivers can't observe a
// half-stamped header. Each method is one assignment because the new BaseEvent
// already carries every routing field — emit builds it once per
// call.

func (e TurnStart) stamp(b BaseEvent) Event       { e.BaseEvent = b; return e }
func (e MessageDelta) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }
func (e ReasoningDelta) stamp(b BaseEvent) Event  { e.BaseEvent = b; return e }
func (e ToolCallStart) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e ToolCallEnd) stamp(b BaseEvent) Event     { e.BaseEvent = b; return e }
func (e PlanGenerated) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e CompactBoundary) stamp(b BaseEvent) Event { e.BaseEvent = b; return e }
func (e MemoryUpdated) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e TurnInterrupted) stamp(b BaseEvent) Event { e.BaseEvent = b; return e }
func (e TurnEnd) stamp(b BaseEvent) Event         { e.BaseEvent = b; return e }
func (e ErrorEvent) stamp(b BaseEvent) Event      { e.BaseEvent = b; return e }

// TurnEndReason enumerates why a turn ended.
type TurnEndReason int

const (
	// TurnEndCompleted — the model returned a stop-marker normally.
	TurnEndCompleted TurnEndReason = iota
	// TurnEndCanceled — the client called [Service.Cancel] or ctx was
	// canceled.
	TurnEndCanceled
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
	case TurnEndCanceled:
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
