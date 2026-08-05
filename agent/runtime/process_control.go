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
	termination    *terminationRequest
	runCancel      atomic.Pointer[context.CancelFunc]
	toolCallCancel atomic.Pointer[context.CancelFunc]
}

type terminationRequest struct {
	reason string
}

func newProcessSignals() processSignals {
	return processSignals{}
}

var _ core.ProcessControl = processControl{}

func (c processControl) Terminate(reason string) {
	c.process.signals.queueTermination(reason)
}

func (c processControl) CancelToolCall() {
	c.process.signals.fireToolCallCancel()
}

// Suspension returns a defensive copy of the resumable continuation currently
// owned by this process.
func (p *Process) Suspension() *interaction.Suspension {
	if p == nil {
		return nil
	}
	return p.state.suspension()
}

func (c processControl) Suspend(ctx context.Context, suspension interaction.Suspension) (core.ActionStatus, error) {
	ctx = normalizeContext(ctx)
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

// queueTermination retains the first pending request so concurrent callers
// cannot overwrite the cause that stopped the process.
func (s *processSignals) queueTermination(reason string) {
	request := terminationRequest{reason: reason}
	s.terminationMu.Lock()
	defer s.terminationMu.Unlock()
	if s.termination == nil {
		s.termination = &request
	}
}

// drainTermination atomically claims the pending request, if any.
func (s *processSignals) drainTermination() *terminationRequest {
	s.terminationMu.Lock()
	defer s.terminationMu.Unlock()
	request := s.termination
	s.termination = nil
	return request
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
