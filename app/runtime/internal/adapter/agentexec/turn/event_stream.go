package turn

import (
	"context"
	"iter"
)

func (s *memoryDispatcher) Events(ctx context.Context, handle TurnHandle) (iter.Seq[Event], error) {
	state, err := s.findTurn(handle.TurnID)
	if err == nil {
		if !state.claimEvents() {
			return nil, ErrTurnNotFound
		}
		return eventSequence(ctx, state), nil
	}
	// A fast turn may finish and leave the live registry after StartTurn returns
	// but before its caller can open Events. The opaque handle retains that
	// exact state, so the first subscriber can still drain its buffered terminal
	// stream. A second subscriber is rejected by claimEvents (single-consumer).
	state = handle.state
	if state == nil ||
		state.handle.SessionID != handle.SessionID ||
		state.handle.TurnID != handle.TurnID ||
		!state.claimEvents() {
		return nil, err
	}
	return eventSequence(ctx, state), nil
}

func eventSequence(ctx context.Context, state *turnState) iter.Seq[Event] {
	// Single-consumer pull stream. The internal select multiplexes the
	// turn's event channel against ctx so the iterator stops promptly
	// when the caller stops listening; even while parked waiting for
	// the next event. runTurn closes state.events on turn end, which
	// terminates the range cleanly (ok == false).
	//
	// Consecutive text deltas (MessageDelta / ReasoningDelta) already buffered
	// on the channel are coalesced into one event before yielding. Under load;
	// the per-token LLM stream running ahead of the SSE consumer; this collapses
	// the 1-token-1-frame volume, cutting the hub's live-event drop rate
	// (hub.go), without touching the durable transcript (item.completed still
	// carries the full text) or adding latency: the drain is non-blocking, so a
	// trickling stream still yields each token the moment it arrives.
	return func(yield func(Event) bool) {
		var spill Event // a different-kind event pulled off mid-coalesce, yielded next
		recv := func() (Event, bool) {
			if spill != nil {
				ev := spill
				spill = nil
				return ev, true
			}
			select {
			case ev, ok := <-state.events:
				return ev, ok
			case <-ctx.Done():
				return nil, false
			}
		}
		for {
			ev, ok := recv()
			if !ok || !yield(coalesceTextDeltas(ev, state.events, &spill)) {
				return
			}
		}
	}
}

// coalesceTextDeltas merges a run of same-kind text deltas (MessageDelta /
// ReasoningDelta) already buffered on ch into head, draining without blocking
// (the default branch = nothing more queued -> stop). A different-kind event
// pulled off mid-drain is parked in *spill for the caller to yield next, so
// ordering is preserved. The merged event keeps the head event's metadata; deltas are
// ephemeral (no SSE id, §5.2), so a merged delta's seq is immaterial.
func coalesceTextDeltas(head Event, ch <-chan Event, spill *Event) Event {
	switch h := head.(type) {
	case MessageDelta:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return h // channel closed; recv() sees the close next and stops
				}
				if d, same := ev.(MessageDelta); same {
					h.Text += d.Text
					continue
				}
				*spill = ev
			default:
			}
			return h
		}
	case ReasoningDelta:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return h
				}
				if d, same := ev.(ReasoningDelta); same {
					h.Text += d.Text
					continue
				}
				*spill = ev
			default:
			}
			return h
		}
	default:
		return head
	}
}
