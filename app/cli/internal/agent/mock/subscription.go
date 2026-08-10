package mock

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) FollowRun(ctx context.Context, in agent.FollowRun) (agent.RunStream, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("mock: %w", err)
	}
	run, fault, err := r.openSubscription(in)
	if err != nil {
		return nil, err
	}
	subscription := &runSubscription{runtime: r, ctx: ctx, run: run, after: in.After, fault: fault}
	return subscription.stream, nil
}

func (r *Runtime) openSubscription(in agent.FollowRun) (*runState, SubscriptionFault, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[in.RunID]
	if !ok {
		return nil, SubscriptionFault{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if in.After < run.startedAfter {
		return nil, SubscriptionFault{}, fmt.Errorf("mock: cursor %d predates run start cursor %d", in.After, run.startedAfter)
	}
	session := r.sessions[run.sessionID]
	latest := agent.Cursor(len(session.events))
	if in.After > latest {
		return nil, SubscriptionFault{}, fmt.Errorf("mock: cursor %d is after session cursor %d", in.After, latest)
	}
	fault, err := r.takeFaultLocked()
	return run, fault, err
}

type runSubscription struct {
	runtime  *Runtime
	ctx      context.Context
	run      *runState
	after    agent.Cursor
	fault    SubscriptionFault
	position int
}

func (s *runSubscription) stream(yield func(agent.Envelope, error) bool) {
	for {
		next, active, changed := s.next()
		if next != nil {
			if !s.deliver(*next, yield) {
				return
			}
			continue
		}
		if !active || !s.awaitChange(changed, yield) {
			return
		}
	}
}

func (s *runSubscription) next() (*agent.Envelope, bool, <-chan struct{}) {
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	session := s.runtime.sessions[s.run.sessionID]
	for _, envelope := range session.events {
		if envelope.Cursor <= s.after || envelope.RunID != s.run.id {
			continue
		}
		cloned := cloneEnvelope(envelope)
		return &cloned, s.run.status == agent.RunActive, session.changed
	}
	return nil, s.run.status == agent.RunActive, session.changed
}

func (s *runSubscription) deliver(next agent.Envelope, yield func(agent.Envelope, error) bool) bool {
	s.position++
	if s.fault.Kind == FaultGap && s.position == s.fault.After {
		s.after = next.Cursor
		return true
	}
	s.after = next.Cursor
	if !yield(next, nil) || !s.injectDeliveredFault(next, yield) {
		return false
	}
	switch next.Event.(type) {
	case agent.RunInterrupted, agent.RunFinished:
		return false
	default:
	}
	if s.fault.Kind == FaultDisconnect && s.position == s.fault.After {
		yield(agent.Envelope{}, fmt.Errorf("%w after cursor %d", agent.ErrDisconnected, s.after))
		return false
	}
	return true
}

func (s *runSubscription) injectDeliveredFault(next agent.Envelope, yield func(agent.Envelope, error) bool) bool {
	if s.position != s.fault.After {
		return true
	}
	switch s.fault.Kind {
	case FaultDuplicate:
		return yield(next, nil)
	case FaultConflict:
		conflict := cloneEnvelope(next)
		conflict.ID += "_conflict"
		yield(conflict, nil)
		return false
	case FaultDisconnect, FaultGap, "":
		return true
	default:
		return true
	}
}

func (s *runSubscription) awaitChange(changed <-chan struct{}, yield func(agent.Envelope, error) bool) bool {
	select {
	case <-changed:
		return true
	case <-s.ctx.Done():
		yield(agent.Envelope{}, context.Cause(s.ctx))
		return false
	}
}

func (r *Runtime) takeFaultLocked() (SubscriptionFault, error) {
	if r.fault >= len(r.Faults) {
		return SubscriptionFault{}, nil
	}
	fault := r.Faults[r.fault]
	r.fault++
	if fault.After < 1 {
		fault.After = 1
	}
	switch fault.Kind {
	case FaultDisconnect, FaultDuplicate, FaultGap, FaultConflict:
		return fault, nil
	default:
		return SubscriptionFault{}, fmt.Errorf("mock: unknown subscription fault %q", fault.Kind)
	}
}
