package runtime

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
)

type processControl struct{ process *Process }

// processSignals owns asynchronous control signals for one Process. Queued
// termination is observed at a process boundary; active run and tool-call
// cancellation are delivered immediately through atomically owned cancel
// functions.
type processSignals struct {
	terminationMu  sync.Mutex
	termination    *core.TerminationSignal
	runCancel      atomic.Pointer[context.CancelFunc]
	toolCallCancel atomic.Pointer[context.CancelFunc]
}

func newProcessSignals() processSignals {
	return processSignals{}
}

var _ core.ProcessControl = processControl{}

func (c processControl) TerminateAgent(reason string) {
	c.process.signals.queueTermination(core.TerminationScopeAgent, reason)
}

func (c processControl) TerminateAction(reason string) {
	c.process.signals.queueTermination(core.TerminationScopeAction, reason)
}

func (c processControl) TerminateToolCall() {
	c.process.signals.fireToolCallCancel()
}

// Suspension returns a defensive copy of the durable continuation currently
// owned by this process.
func (p *Process) Suspension() *interaction.Suspension {
	if p == nil {
		return nil
	}
	return p.state.suspension()
}

func (c processControl) Suspend(ctx context.Context, suspension interaction.Suspension) (core.ActionStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	process := c.process
	nested, err := process.prepareNestedSuspension(suspension)
	if err != nil {
		return core.ActionFailed, err
	}
	if err := process.state.parkSuspension(suspension); err != nil {
		return core.ActionFailed, err
	}
	process.commitNestedSuspension(nested)
	process.publishEvent(ctx, event.ProcessWaiting{Header: process.eventHeader(), Suspension: process.Suspension()})
	return core.ActionWaiting, nil
}

// queueTermination merges a request into the pending signal. Agent-wide
// termination always outranks action-level replanning; the first reason at the
// winning scope is retained so concurrent callers cannot overwrite causality.
func (s *processSignals) queueTermination(scope core.TerminationScope, reason string) {
	signal := core.TerminationSignal{Scope: scope, Reason: reason}
	s.terminationMu.Lock()
	defer s.terminationMu.Unlock()
	if s.termination == nil || (s.termination.Scope == core.TerminationScopeAction && scope == core.TerminationScopeAgent) {
		s.termination = &signal
	}
}

// drainTerminate atomically claims the merged pending signal, if any.
func (s *processSignals) drainTerminate() *core.TerminationSignal {
	s.terminationMu.Lock()
	defer s.terminationMu.Unlock()
	signal := s.termination
	s.termination = nil
	return signal
}

// fireRunCancel cancels the active Run or Continue context, if any.
func (s *processSignals) fireRunCancel() {
	cancel := s.runCancel.Load()
	if cancel == nil || *cancel == nil {
		return
	}
	(*cancel)()
}

// registerRunCancel installs the cancel function for one active Run or
// Continue invocation and returns an ownership-safe release closure.
func (s *processSignals) registerRunCancel(cancel context.CancelFunc) (release func()) {
	cell := &cancel
	s.runCancel.Store(cell)
	return func() {
		s.runCancel.CompareAndSwap(cell, nil)
	}
}

// fireToolCallCancel cancels the active tool call, if any.
func (s *processSignals) fireToolCallCancel() {
	cancel := s.toolCallCancel.Load()
	if cancel == nil || *cancel == nil {
		return
	}
	(*cancel)()
}

// registerToolCallCancel installs a fresh cancel function and returns a
// release closure that clears it only while it still owns the slot.
func (s *processSignals) registerToolCallCancel(cancel context.CancelFunc) (release func()) {
	cell := &cancel
	s.toolCallCancel.Store(cell)
	return func() {
		s.toolCallCancel.CompareAndSwap(cell, nil)
	}
}
