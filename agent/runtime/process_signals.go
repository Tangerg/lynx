package runtime

import (
	"context"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
)

// processSignals owns the external control signals an Process
// uses to coordinate with the outside world:
//
//   - terminate         buffered channel of TerminationSignal —
//     Tick consumes one at the next boundary. Scope
//     Agent stops the process; Action triggers a
//     re-plan; ToolCall is fired immediately (see
//     toolCallCancel below) rather than queued.
//   - toolCallCancel    atomic.Pointer storing the cancel func of the
//     most recently derived tool-call context.
//     TerminateToolCall fires it; the in-flight tool
//     observes ctx.Done() and aborts.
//
// The struct intentionally has no methods that need an outer lock —
// everything is built on channel + atomic primitives so signaling can
// race with state-machine reads without contending the main mutex.
type processSignals struct {
	terminate      chan core.TerminationSignal
	runCancel      atomic.Pointer[context.CancelFunc]
	toolCallCancel atomic.Pointer[context.CancelFunc]
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

// fireRunCancel cancels the currently active Run / Continue context, if any.
// Engine.Kill uses it so an in-flight action, provider call, or synchronous
// child does not have to reach another process checkpoint before stopping.
func (s *processSignals) fireRunCancel() {
	cancel := s.runCancel.Load()
	if cancel == nil || *cancel == nil {
		return
	}
	(*cancel)()
}

// registerRunCancel installs the cancel function for one active Run /
// Continue invocation and returns an ownership-safe release closure.
func (s *processSignals) registerRunCancel(cancel context.CancelFunc) (release func()) {
	cell := &cancel
	s.runCancel.Store(cell)
	return func() {
		s.runCancel.CompareAndSwap(cell, nil)
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
