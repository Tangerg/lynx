package turn

import (
	"context"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func (s *controller) Events(ctx context.Context, handle Handle) (iter.Seq[runs.ExecutorEvent], error) {
	state, err := s.findTurn(handle.TurnID)
	if err == nil {
		if !state.claimEvents() {
			return nil, ErrTurnNotFound
		}
		return eventSequence(ctx, state), nil
	}
	// A fast turn may finish and leave the live registry after StartTurn returns
	// but before its caller can open Events. The opaque handle retains that
	// exact state, so one late consumer can still drain its buffered terminal
	// stream.
	state = handle.state
	if state == nil ||
		state.handle.SessionID != handle.SessionID ||
		state.handle.TurnID != handle.TurnID ||
		!state.claimEvents() {
		return nil, err
	}
	return eventSequence(ctx, state), nil
}

func eventSequence(ctx context.Context, state *turnState) iter.Seq[runs.ExecutorEvent] {
	// Single-active-consumer pull stream. The internal select multiplexes the
	// turn's event channel against ctx so the iterator stops promptly
	// when the caller stops listening, even while parked waiting for
	// the next event. runTurn closes state.events on turn end, which
	// terminates the range cleanly (ok == false). Releasing the consumer when
	// this sequence returns lets a continuation segment attach to a parked turn.
	//
	// Consecutive text deltas (MessageDelta / ReasoningDelta) already buffered
	// on the channel are coalesced into one event before yielding. When the
	// per-token LLM stream runs ahead of the SSE consumer, this collapses the
	// one-token-per-frame volume and cuts the downstream live-event drop rate.
	// It does not touch the durable transcript or add latency: the drain is
	// non-blocking, so a trickling stream still yields each token immediately.
	return func(yield func(runs.ExecutorEvent) bool) {
		defer state.releaseEvents()
		var spill *runs.ExecutorEvent // a different-kind event pulled off mid-coalesce, yielded next
		recv := func() (runs.ExecutorEvent, bool) {
			if spill != nil {
				ev := spill
				spill = nil
				return *ev, true
			}
			select {
			case ev, ok := <-state.events:
				return ev, ok
			case <-ctx.Done():
				return runs.ExecutorEvent{}, false
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
// ephemeral (no SSE id, §5.2), so merged delta boundaries are immaterial.
func coalesceTextDeltas(head runs.ExecutorEvent, ch <-chan runs.ExecutorEvent, spill **runs.ExecutorEvent) runs.ExecutorEvent {
	switch h := head.Payload.(type) {
	case runs.MessageDelta:
		var merged strings.Builder
	messageDeltas:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					break messageDeltas
				}
				if d, same := ev.Payload.(runs.MessageDelta); same && ev.Source == head.Source {
					if merged.Len() == 0 {
						merged.Grow(len(h.Text) + len(d.Text))
						merged.WriteString(h.Text)
					}
					merged.WriteString(d.Text)
					continue
				}
				spillEvent := ev
				*spill = &spillEvent
			default:
			}
			break
		}
		if merged.Len() > 0 {
			h.Text = merged.String()
		}
		head.Payload = h
		return head
	case runs.ReasoningDelta:
		var merged strings.Builder
	reasoningDeltas:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					break reasoningDeltas
				}
				if d, same := ev.Payload.(runs.ReasoningDelta); same && ev.Source == head.Source {
					if merged.Len() == 0 {
						merged.Grow(len(h.Text) + len(d.Text))
						merged.WriteString(h.Text)
					}
					merged.WriteString(d.Text)
					continue
				}
				spillEvent := ev
				*spill = &spillEvent
			default:
			}
			break
		}
		if merged.Len() > 0 {
			h.Text = merged.String()
		}
		head.Payload = h
		return head
	default:
		return head
	}
}
