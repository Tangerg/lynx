package turn

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// Event is the sealed sum type emitted on a turn's event channel.
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
// The unexported stamp method keeps the sum type closed to this package.
//
// Every event is also an [execution.Event]: it carries the run-lifecycle
// classification the application pipeline switches on (terminal / park). Most
// events are neither — BaseEvent supplies the defaults, only the two lifecycle
// events override — so the run coordinator drives the run off the domain
// contract without depending on this package's concrete types.
type Event interface {
	execution.Event
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
	// distinct "denied" terminal — see [agentexec.ErrToolDenied].
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
// [Dispatcher.Resume], which continues the same turn (its events resume on
// the same channel). Carries the pending interrupt(s).
type TurnInterrupted struct {
	BaseEvent
	Interrupts []Interrupt
}

// Interrupt is one pending HITL request surfaced by [TurnInterrupted].
// Kind is the wire interrupt kind (API.md §6: "approval" | "question" |
// "toolResult") so it lines up with what the client declares in
// ClientCapabilities.interruptTypes. Payload is the awaitable's prompt —
// an [ApprovalPrompt] for a gated tool call ("approval"), or a question
// prompt for a tool asking the user ("question").
type Interrupt struct {
	Kind    string // "approval" | "question"
	Payload any
}

// TurnEnd fires once at the end of a turn. Reason is the domain terminal
// [execution.Outcome] (the single terminal-reason taxonomy — an interrupt is
// NOT one, parking is a separate state); TokenUsage / CostUSD are the rolled-up
// totals for the turn (sum across every LLM call inside it).
type TurnEnd struct {
	BaseEvent
	Reason       execution.Outcome
	TokenUsage   accounting.TokenUsage
	UsageByModel []accounting.ModelUsage // per-model breakdown; one entry for a single-model turn
	CostUSD      float64                 // turn cost; zero unless a pricing hook is configured (engine.Config.Pricing)
	Duration     time.Duration
	// MaxBudget / MaxCostUSD / MaxSteps echo the turn's configured caps so a
	// Reason=[execution.OutcomeMaxBudget] / [execution.OutcomeMaxSteps] terminal
	// can be described precisely ("spent $4.20 of $4.00 budget" / "reached the
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
// Ephemeral by nature; transport maps it to a segment.progress usage preview.
// CostUSD is zero unless a pricing hook is configured.
type UsageReported struct {
	BaseEvent
	TokenUsage accounting.TokenUsage
	CostUSD    float64
	// ContextTokens is this round's prompt-token count — the live context-window
	// occupancy (not the cumulative roll-up in TokenUsage). Transport maps it to
	// segment.progress.contextTokens for the client's occupancy gauge.
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
// [Dispatcher.InjectSteering] while the turn was looping) is consumed into the
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

// Terminal and Interrupt classify an event for the run pipeline
// ([execution.Event]). By default an event neither ends nor parks the run;
// BaseEvent carries those defaults so every concrete event satisfies the
// contract, and only the two lifecycle events override: TurnEnd ends the run
// with its [execution.Outcome], TurnInterrupted parks it (an ErrorEvent is a
// pre-terminal record — the following TurnEnd carries OutcomeError).
func (BaseEvent) Terminal() (execution.Outcome, bool) { return 0, false }
func (BaseEvent) Interrupt() bool                     { return false }

func (e TurnEnd) Terminal() (execution.Outcome, bool) { return e.Reason, true }
func (TurnInterrupted) Interrupt() bool               { return true }
