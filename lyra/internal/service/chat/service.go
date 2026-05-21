// Package chat defines the ChatService — Lyra's one-turn dispatch
// surface. A turn is the unit of interaction: client sends one
// message, runtime drives one (possibly multi-tool) round, runtime
// streams events back, turn ends with a [TurnEnd] event.
package chat

import (
	"context"
	"time"
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
//	    case TurnEnd: return
//	    case Error: handleErr(e)
//	    }
//	}
//
// Cancellation flows through ctx — closing ctx cancels the turn and
// drains the event channel.
type Service interface {
	// StartTurn launches a new turn against the given session. Returns
	// a handle the caller uses to subscribe to events. The method
	// returns as soon as the turn is scheduled — actual LLM work
	// happens asynchronously and surfaces via [Events].
	StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error)

	// Events returns the read-only channel for a turn's events. The
	// channel closes when the turn ends (success or error). Calling
	// Events twice for the same turn returns two independent channels
	// that fan-out from the same underlying stream — useful for
	// transport layers that need to multiplex.
	Events(ctx context.Context, handle TurnHandle) (<-chan Event, error)

	// InjectSteering delivers a user message mid-turn. The runtime
	// queues it until the next tool boundary then injects it into the
	// model's context. No-op when the turn has already completed.
	// (M-future; signature reserved to stabilize the surface.)
	InjectSteering(ctx context.Context, handle TurnHandle, message string) error

	// Cancel stops the turn immediately, drains pending tool calls
	// safely, and emits a final [TurnEnd] event with Reason=Cancelled.
	Cancel(ctx context.Context, handle TurnHandle) error
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
type Event interface {
	isLyraEvent()
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

// TurnEnd fires once at the end of a turn. Reason explains why the
// turn ended; TokenUsage / CostUSD are the rolled-up totals for the
// turn (sum across every LLM call inside it).
type TurnEnd struct {
	BaseEvent
	Reason     TurnEndReason
	TokenUsage TokenUsage
	CostUSD    float64
	Duration   time.Duration
}

// ErrorEvent fires when an unrecoverable error aborts the turn. The
// turn channel still closes after this event so receivers don't need
// to special-case the end.
type ErrorEvent struct {
	BaseEvent
	Message string
	Code    string // stable error code; see errors.go
}

// TurnEndReason enumerates why a turn ended.
type TurnEndReason int

const (
	// TurnEndCompleted — the model returned a stop-marker normally.
	TurnEndCompleted TurnEndReason = iota
	// TurnEndCancelled — the client called [Service.Cancel] or ctx was
	// cancelled.
	TurnEndCancelled
	// TurnEndErrored — the turn aborted on error. An [ErrorEvent]
	// fires before [TurnEnd] in this case.
	TurnEndErrored
)

func (r TurnEndReason) String() string {
	switch r {
	case TurnEndCompleted:
		return "completed"
	case TurnEndCancelled:
		return "cancelled"
	case TurnEndErrored:
		return "errored"
	default:
		return "unknown"
	}
}

// TokenUsage is the per-turn token roll-up. Field names mirror lynx
// core.LLMInvocation so future transport adapters can map 1:1.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
}
