package mock

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) SubscribeRun(ctx context.Context, in agent.SubscribeRun) (agent.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return agent.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.SegmentStream{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return agent.SegmentStream{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if run.active != in.SegmentID || run.status != agent.RunStatusRunning {
		return agent.SegmentStream{}, fmt.Errorf("%w: run %s is not executing segment %s", agent.ErrStaleSegment, in.RunID, in.SegmentID)
	}
	segment := run.segments[in.SegmentID]
	if segment == nil {
		return agent.SegmentStream{}, fmt.Errorf("%w: %s", agent.ErrStaleSegment, in.SegmentID)
	}

	head := len(segment.events)
	start := head // Empty checkpoint attaches at the current head.
	if in.AfterEventID != "" {
		at := replayIndex(segment.events, in.AfterEventID)
		if at < 0 {
			return agent.SegmentStream{}, fmt.Errorf("%w: event %s", agent.ErrReplayUnavailable, in.AfterEventID)
		}
		start = at + 1
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		return agent.SegmentStream{}, err
	}
	return r.bindSegmentLocked(ctx, run, segment, start, head, "", fault), nil
}

func replayIndex(events []agent.RunEvent, eventID string) int {
	for i, event := range events {
		if event.EventID == eventID && agent.ReplayableEvent(event.Event) {
			return i
		}
	}
	return -1
}

func (r *Runtime) openSegmentLocked(run *runState) *segmentState {
	r.next++
	segment := &segmentState{id: fmt.Sprintf("seg_mock_%d", r.next), changed: make(chan struct{})}
	run.active = segment.id
	run.segments[segment.id] = segment
	return segment
}

func (r *Runtime) bindSegmentLocked(
	ctx context.Context,
	run *runState,
	segment *segmentState,
	start int,
	replayUntil int,
	userItemID string,
	fault SubscriptionFault,
) agent.SegmentStream {
	headEventID := ""
	for i := len(segment.events) - 1; i >= 0; i-- {
		if agent.ReplayableEvent(segment.events[i].Event) {
			headEventID = segment.events[i].EventID
			break
		}
	}
	subscription := &segmentSubscription{
		runtime: r, ctx: ctx, run: run, segment: segment,
		next: start, replayUntil: replayUntil, fault: fault,
	}
	return agent.SegmentStream{
		RunID: run.id, SegmentID: segment.id, UserItemID: userItemID,
		HeadEventID: headEventID, Events: subscription.stream,
	}
}

type segmentSubscription struct {
	runtime     *Runtime
	ctx         context.Context
	run         *runState
	segment     *segmentState
	next        int
	replayUntil int
	fault       SubscriptionFault
	position    int
}

func (s *segmentSubscription) stream(yield func(agent.RunEvent, error) bool) {
	for {
		next, closed, changed := s.nextEvent()
		if next != nil {
			if !s.deliver(*next, yield) {
				return
			}
			continue
		}
		if closed || !s.awaitChange(changed, yield) {
			return
		}
	}
}

func (s *segmentSubscription) nextEvent() (*agent.RunEvent, bool, <-chan struct{}) {
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	for s.next < len(s.segment.events) {
		at := s.next
		s.next++
		event := s.segment.events[at]
		if at < s.replayUntil && !agent.ReplayableEvent(event.Event) {
			continue
		}
		cloned := event.Clone()
		return &cloned, s.segment.closed, s.segment.changed
	}
	return nil, s.segment.closed, s.segment.changed
}

func (s *segmentSubscription) deliver(next agent.RunEvent, yield func(agent.RunEvent, error) bool) bool {
	s.position++
	if !yield(next, nil) {
		return false
	}
	if s.position == s.fault.After {
		switch s.fault.Kind {
		case FaultDuplicate:
			if !yield(next, nil) {
				return false
			}
		case FaultConflict:
			conflict := next.Clone()
			conflict.Event = agent.BlockCompleted{Block: agent.Block{ID: "conflict", RunID: next.RunID, Status: agent.BlockStatusCompleted, Kind: agent.BlockNotice, Text: "conflicting replay"}}
			yield(conflict, nil)
			return false
		case FaultDisconnect:
			yield(agent.RunEvent{}, fmt.Errorf("%w after event %s", agent.ErrDisconnected, next.EventID))
			return false
		}
	}
	return true
}

func (s *segmentSubscription) awaitChange(changed <-chan struct{}, yield func(agent.RunEvent, error) bool) bool {
	select {
	case <-changed:
		return true
	case <-s.ctx.Done():
		yield(agent.RunEvent{}, context.Cause(s.ctx))
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
	case FaultDisconnect, FaultDuplicate, FaultConflict:
		return fault, nil
	default:
		return SubscriptionFault{}, fmt.Errorf("mock: unknown subscription fault %q", fault.Kind)
	}
}
