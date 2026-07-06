// Package turn defines the turn-dispatch Service — Lyra's one-turn
// surface. A turn is the unit of interaction: client sends one
// message, runtime drives one (possibly multi-tool) round, runtime
// streams events back, turn ends with a [TurnEnd] event.
package turn

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/core/media"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// clientResolver resolves a per-turn chat client for an explicit
// (provider, model) — the seam the multi-provider runtime plugs in so a turn
// can run against a model other than the default. Unexported: turn's own
// consumer-side abstraction, satisfied implicitly by the runtime's provider
// registry (nothing outside this package names it). Returns an error when the
// provider isn't configured / enabled.
type clientResolver interface {
	ResolveClient(ctx context.Context, provider, model string) (*corechat.Client, error)
}

// ErrInputRequired reports that a turn has neither text nor media to send.
var ErrInputRequired = errors.New("turn: input required")

// ErrIncompleteModelSelection reports a provider/model pair where only one side
// was supplied. Turn model selection is explicit: both are set, or both empty.
var ErrIncompleteModelSelection = errors.New("turn: incomplete model selection")

// ErrUnsupportedMedia reports media that the selected model cannot accept.
var ErrUnsupportedMedia = errors.New("turn: unsupported media")

// ErrInvalidTurnLimit reports a negative turn budget / step cap. Limits use
// zero as "unlimited", so negative values have no domain meaning.
var ErrInvalidTurnLimit = errors.New("turn: invalid limit")

// StartTurnRequest is the input to [Service.StartTurn]. SessionID
// binds the turn to its conversation; Message is the user's input.
type StartTurnRequest struct {
	SessionID string
	Message   string

	// Media carries the turn's image attachments (runs.start input image
	// blocks). Nil for a text-only turn. They ride the user message to the
	// model as UserMessage.Media; only models whose catalog modalities accept
	// image input should be sent them (gated before StartTurn).
	Media []*media.Media

	// Cwd is the session's working directory — the project root the turn's
	// filesystem + shell tools run in. Resolved from Session.cwd by the
	// caller (runs.start). Empty falls back to the engine's default workdir.
	Cwd string

	// Provider + Model select the model this turn runs against (the wire
	// runs.start{providerId, model}). Both empty uses the runtime's default;
	// both set resolves that provider+model client via the clientResolver and
	// runs the turn against it. The provider is explicit — never inferred.
	Provider string
	Model    string

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

	// MaxSteps caps the turn's tool-call rounds (model turns); 0 = unlimited.
	// On overrun the turn stops cleanly after the round with
	// Reason=[TurnEndStepsExceeded] (distinct from the token/cost budget).
	MaxSteps int
}

// Validate rejects malformed turn drafts before they bind to a session or
// launch an agent process.
func (r StartTurnRequest) Validate() error {
	if r.Message == "" && len(r.Media) == 0 {
		return ErrInputRequired
	}
	if (r.Model == "") != (r.Provider == "") {
		return ErrIncompleteModelSelection
	}
	if r.MaxBudget < 0 {
		return fmt.Errorf("%w: MaxBudget must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxCostUSD < 0 {
		return fmt.Errorf("%w: MaxCostUSD must be non-negative", ErrInvalidTurnLimit)
	}
	if r.MaxSteps < 0 {
		return fmt.Errorf("%w: MaxSteps must be non-negative", ErrInvalidTurnLimit)
	}
	return nil
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
// rebinds chat history; Approved is the human decision delivered to the
// re-parked process.
type RehydrateRequest struct {
	SessionID string
	ProcessID string
	Approved  bool

	// Provider + Model are the parked run's per-run model selection, persisted
	// on the interrupt. Both set re-resolves that client so the continuation
	// runs against the SAME model it parked on; both empty (or no resolver)
	// runs on the platform default. The provider is explicit — never inferred.
	Provider string
	Model    string
}

// Service is the turn-dispatch contract.
//
// A typical interaction:
//
//	handle, err := turn.StartTurn(ctx, req)
//	events := turn.Events(ctx, handle)
//	for ev := range events {
//	    switch e := ev.(type) {
//	    case MessageDelta: ui.AppendText(e.Text)
//	    case ToolCallStart: ui.ShowSpinner(e.ToolName)
//	    case TurnEnd: return
//	    case Error: handleErr(e)
//	    }
//	}
//
// A turn parked on a HITL interrupt pauses after [TurnInterrupted]; call
// [Resume] with the user's decision to continue the same turn.
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
	// call awaiting approval, or an ask_user / exit_plan_mode question —
	// all surface as a [TurnInterrupted] event). The structured
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

	// ForgetSession releases the process-local state the service keeps keyed by
	// a session — currently the SessionStart fire-once gate. Call it when a
	// session is deleted: its id (a UUID) never returns, so the gate entry is
	// dead weight, and without eviction the set grows by one entry per session
	// the process ever ran a turn for. A no-op for a session never seen.
	ForgetSession(sessionID string)
}

// ------------------------------------------------------------------
// Event types
// ------------------------------------------------------------------

// Event is the sealed sum type emitted on a turn's event channel.
// Concrete event types implement this marker so callers can type-switch.
//
// All events carry [BaseEvent] for routing — SessionID + TurnID + Seq
// + Timestamp. Seq is a gapless per-turn counter assigned atomically at
// emit — but it is NOT an arrival-order guarantee: parallel tool calls emit
// from concurrent goroutines, so two can take Seq n and n+1 and then send in
// either order. The durable, arrival-ordered identity is the wire eventId the
// single-consumer pump assigns (the Last-Event-Id / gap-detection authority);
// Seq is for intra-turn correlation, not for detecting gaps across reordered
// arrivals.
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
	// SafetyClass is the tool's wire safety class ("safe"|"write"|"exec"),
	// stamped on the live toolCall Item so a client shows its risk class
	// without joining tools.list.
	SafetyClass string
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
	// distinct "denied" terminal — see [kernel.ErrToolDenied].
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
// model): a gated tool call needs approval, or a tool (ask_user /
// exit_plan_mode) asks the user a question. The turn does NOT end — it
// suspends at [core.StatusWaiting]; the client answers via
// [Service.Resume], which continues the same turn (its events resume on
// the same channel). Carries the pending interrupt(s).
type TurnInterrupted struct {
	BaseEvent
	Interrupts []Interrupt
}

// Interrupt is one pending HITL request surfaced by [TurnInterrupted].
// Kind is the wire interrupt kind (API.md §6: "approval" | "question" |
// "toolResult") so it lines up with what the client negotiates in
// ClientCapabilities.InterruptKinds. Payload is the awaitable's prompt —
// an [ApprovalPrompt] for a gated tool call ("approval"), or a question
// prompt for a tool asking the user ("question").
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
	// MaxBudget / MaxCostUSD / MaxSteps echo the turn's configured caps so a
	// Reason=TurnEndBudgetExceeded / TurnEndStepsExceeded terminal can be
	// described precisely ("spent $4.20 of $4.00 budget" / "reached the
	// 8000-token budget" / "reached the 8-step limit"). Zero when uncapped.
	MaxBudget  int64
	MaxCostUSD float64
	MaxSteps   int
}

// ErrorEvent fires when an unrecoverable error aborts the turn. The
// turn channel still closes after this event so receivers don't need
// to special-case the end.
type ErrorEvent struct {
	BaseEvent
	Message string
	Code    string // stable error code; see errors.go
}

// UsageReported fires once per completed LLM round with the turn's
// cumulative token roll-up + cost so far — the mid-run "tokens / cost spent"
// readout (the live preview whose authoritative final lands on [TurnEnd]).
// Ephemeral by nature; transport maps it to a run.progress usage preview.
// CostUSD is zero unless a pricing hook is configured.
type UsageReported struct {
	BaseEvent
	TokenUsage TokenUsage
	CostUSD    float64
	// ContextTokens is this round's prompt-token count — the live context-window
	// occupancy (not the cumulative roll-up in TokenUsage). Transport maps it to
	// run.progress.contextTokens for the client's occupancy gauge.
	ContextTokens int64
}

// TodosUpdated fires after the model rewrites its task list (the todo_write
// tool) with the full new list, so transport can project it to a
// state.snapshot{todos} and a client renders the task panel — the model's
// checklist becomes a first-class surface, not a model-only side effect.
type TodosUpdated struct {
	BaseEvent
	Todos []todo.Item
}

// SteerMessage fires when a mid-run steering message (injected via
// [Service.InjectSteering] while the turn was looping) is consumed into the
// running loop — between the round that just finished and the next one. The
// transport surfaces it as a userMessage Item so the steered turn shows on the
// timeline and lands in the durable transcript, exactly like the opening user
// turn (it's also already in the model's context, injected into the loop).
type SteerMessage struct {
	BaseEvent
	Text string
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
func (e CompactBoundary) stamp(b BaseEvent) Event { e.BaseEvent = b; return e }
func (e MemoryUpdated) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e TurnInterrupted) stamp(b BaseEvent) Event { e.BaseEvent = b; return e }
func (e TurnEnd) stamp(b BaseEvent) Event         { e.BaseEvent = b; return e }
func (e ErrorEvent) stamp(b BaseEvent) Event      { e.BaseEvent = b; return e }
func (e UsageReported) stamp(b BaseEvent) Event   { e.BaseEvent = b; return e }
func (e SteerMessage) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }
func (e TodosUpdated) stamp(b BaseEvent) Event    { e.BaseEvent = b; return e }

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
	// TurnEndStepsExceeded — the turn hit [StartTurnRequest.MaxSteps] (the
	// tool-call-round cap) and stopped cleanly after the round. Not an error;
	// distinct from the token/cost budget so the wire can surface the dedicated
	// maxSteps outcome.
	TurnEndStepsExceeded
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
	case TurnEndStepsExceeded:
		return "steps_exceeded"
	default:
		return "unknown"
	}
}

// TokenUsage is the per-turn token roll-up. Alias for
// [kernel.TokenUsage] — the engine owns the canonical shape and
// the turn package re-exports it so transport adapters can stay
// scoped to one import.
type TokenUsage = kernel.TokenUsage

// ModelUsage is the per-model token breakdown re-exported from the
// engine, so transport adapters consuming [TurnEnd.UsageByModel] stay
// scoped to one import.
type ModelUsage = kernel.ModelUsage
