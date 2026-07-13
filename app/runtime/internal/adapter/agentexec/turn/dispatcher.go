// Package turn defines the turn dispatcher — Lyra's one-turn
// surface. A turn is the unit of interaction: client sends one
// message, runtime drives one (possibly multi-tool) round, runtime
// streams events back, turn ends with a [TurnEnd] event.
package turn

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
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

// Dispatcher is the live-turn dispatch contract.
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
// A turn outlives the ctx that started it: StartTurn keeps the caller's values
// but detaches its cancellation, so an RPC returning does not kill the
// in-flight turn. Call [Dispatcher.Cancel] to stop one turn or [Dispatcher.Close]
// to stop the process-scoped dispatcher.
type Dispatcher interface {
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
	// [Dispatcher.Cancel] to stop the turn itself). Returns [ErrTurnNotFound]
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
	// [Events] again after Resume to drain it. interruptKinds replaces the
	// per-turn HITL surface for the continuation. Returns [ErrTurnNotFound]
	// when the turn isn't parked.
	Resume(ctx context.Context, handle TurnHandle, resolution interrupts.Resolution, interruptKinds []string) error

	// ProcessID returns the agent-process id backing a live (parked) turn
	// — the snapshot key the runtime records so a restart can rebuild the
	// process via [Rehydrate]. Returns [ErrTurnNotFound] when the turn
	// isn't live, or an error when it hasn't dispatched a process yet.
	ProcessID(ctx context.Context, handle TurnHandle) (string, error)

	// Rehydrate rebuilds a parked turn whose live in-memory state was lost from
	// the persisted process snapshot identified by req.ProcessID. It deliberately
	// does not deliver a decision: the caller first attaches Events and commits
	// the durable resume boundary, then calls Resume.
	Rehydrate(ctx context.Context, req RehydrateRequest) (TurnHandle, error)

	// Cancel stops the turn immediately, drains pending tool calls
	// safely, and emits a final [TurnEnd] event with Reason=Canceled.
	Cancel(ctx context.Context, handle TurnHandle) error

	// Close rejects new turns, cancels every live turn (including parked
	// interrupts), and waits up to the dispatcher's shutdown budget for their
	// terminal teardown. It is idempotent. A timeout is reported as
	// [ErrCloseTimeout]; later calls can finish joining the same turn set.
	Close() error

	// ForgetSession releases the process-local state the dispatcher keeps keyed by
	// a session — currently the SessionStart fire-once gate. Call it when a
	// session is deleted: its id (a UUID) never returns, so the gate entry is
	// dead weight, and without eviction the set grows by one entry per session
	// the process ever ran a turn for. A no-op for a session never seen.
	ForgetSession(sessionID string)
}
