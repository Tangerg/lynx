package runtime

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
)

// processSignals owns the three channels / atomic slots an AgentProcess
// uses to coordinate with the outside world:
//
//   - terminate         buffered channel of TerminationSignal —
//     Tick consumes one at the next boundary. Scope
//     Agent stops the process; Action triggers a
//     re-plan; ToolCall is fired immediately (see
//     toolCallCancel below) rather than queued.
//   - pendingAwaitable  atomic.Pointer parking spot for the typed-action
//     AwaitInput / Platform.ResumeProcess handshake.
//     Swap-based ownership: ResumeProcess atomically
//     claims the slot before invoking the handler.
//   - toolCallCancel    atomic.Pointer storing the cancel func of the
//     most recently derived tool-call context.
//     TerminateToolCall fires it; the in-flight tool
//     observes ctx.Done() and aborts.
//
// The struct intentionally has no methods that need an outer lock —
// everything is built on channel + atomic primitives so signaling can
// race with state-machine reads without contending the main mutex.
type processSignals struct {
	terminate        chan core.TerminationSignal
	pendingAwaitable atomic.Pointer[awaitSlot]
	toolCallCancel   atomic.Pointer[context.CancelFunc]
}

// awaitSlot is the parking spot used by the AwaitInput / ResumeProcess
// pair: AwaitInput stores the awaitable here and flips the process to
// StatusWaiting; ResumeProcess swaps the slot out and routes the
// response through awaitable.OnResponseAny so the user-supplied
// handler runs (typically mutating the blackboard). The single-field
// wrapper exists so atomic.Pointer can park an interface value
// (Go won't let us use atomic.Pointer[core.Awaitable] directly with
// concrete-typed Stores).
type awaitSlot struct {
	awaitable core.Awaitable
}

func newProcessSignals() processSignals {
	return processSignals{
		terminate: make(chan core.TerminationSignal, 1),
	}
}

// queueTermination writes a signal to the terminate channel without
// blocking. A duplicate while one is already pending is silently
// dropped — Tick will see the pending one at the next boundary.
func (s *processSignals) queueTermination(scope core.TerminationScope, reason string) {
	signal := core.TerminationSignal{Scope: scope, Reason: reason}
	select {
	case s.terminate <- signal:
	default:
	}
}

// drainTerminate pulls a signal off the channel without blocking. nil
// result means no signal pending.
func (s *processSignals) drainTerminate() *core.TerminationSignal {
	select {
	case sig := <-s.terminate:
		return &sig
	default:
		return nil
	}
}

// fireToolCallCancel cancels the in-flight tool call, if any.
func (s *processSignals) fireToolCallCancel() {
	cancel := s.toolCallCancel.Load()
	if cancel == nil || *cancel == nil {
		return
	}
	(*cancel)()
}

// registerToolCallCancel installs a fresh cancel func and returns a
// release closure that detaches it. A new registration replaces any
// previously-stored one (the old context becomes orphaned — its owning
// action body should already be done by the time a new tool call
// starts).
func (s *processSignals) registerToolCallCancel(cancel context.CancelFunc) (release func()) {
	cell := &cancel
	s.toolCallCancel.Store(cell)
	return func() {
		// Only clear if the slot is still owned — a newer registration
		// would have replaced the current owner, which must not be stomped.
		s.toolCallCancel.CompareAndSwap(cell, nil)
	}
}

// parkAwaitable stores the request as the pending awaitable. Returns
// [core.ActionWaiting] on success or [core.ActionFailed] for nil
// requests. The caller is responsible for publishing
// [event.ProcessWaiting].
func (s *processSignals) parkAwaitable(req core.Awaitable) core.ActionStatus {
	if req == nil {
		return core.ActionFailed
	}
	s.pendingAwaitable.Store(&awaitSlot{awaitable: req})
	return core.ActionWaiting
}

// peekAwaitable returns the parked awaitable without consuming it, or
// nil when nothing is parked. Used by callers that need to inspect a
// suspended process (e.g. supervisor patterns wanting to surface the
// child's pending request back to the LLM).
func (s *processSignals) peekAwaitable() core.Awaitable {
	slot := s.pendingAwaitable.Load()
	if slot == nil {
		return nil
	}
	return slot.awaitable
}

// deliverResponse atomically claims the parked slot and forwards the
// response to the awaitable's typed handler. Returns an error when no
// slot is parked or the response value doesn't match the awaitable's
// expected type.
func (s *processSignals) deliverResponse(response any) (core.ResponseImpact, error) {
	slot := s.pendingAwaitable.Swap(nil)
	if slot == nil {
		return core.ImpactUnchanged, errors.New("runtime.processSignals.deliverResponse: no awaitable response is pending")
	}
	return slot.awaitable.OnResponseAny(response)
}
