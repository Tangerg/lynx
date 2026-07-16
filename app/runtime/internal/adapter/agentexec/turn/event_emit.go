package turn

import "time"

// emit stamps the event with the next sequence number and timestamp and pushes
// it onto the turn's channel. Type-specific stamping lives on each concrete
// event (via the unexported [Event.stamp] method) so this dispatcher stays
// open-closed — adding a new event variant means writing the struct + one stamp
// method, nothing here.
//
// Sends block when the consumer falls behind: the durable history (items.list)
// is built from this stream, so backpressure — the turn slowing to the
// consumer's persistence speed — is correct where dropping would silently
// corrupt persisted items (a lost MessageDelta truncates the item text; a lost
// TurnEnd misreports the outcome as canceled). The turn-lifetime ctx is the
// escape hatch: a canceled turn stops blocking producers even when no consumer
// is left to drain.
func (s *memoryDispatcher) emit(st *turnState, ev Event) bool {
	st.eventMu.Lock()
	defer st.eventMu.Unlock()
	if st.eventsClosed {
		return false
	}
	st.seq++
	stamped := ev.WithMeta(EventMeta{
		SessionID: st.handle.SessionID,
		TurnID:    st.handle.TurnID,
		Seq:       st.seq,
		Timestamp: time.Now(),
	})
	// Prefer delivery: when the buffer has room the event lands regardless of
	// whether the turn ctx was already canceled. This is what makes a canceled
	// turn's TERMINAL event (TurnEnd / the ErrorEvent before it) reach a
	// consumer still draining the stream — Cancel cancels st.ctx *before* the
	// finishTurn / drive path emits the terminal, so a bare select would race
	// the terminal into the ctx.Done() escape and drop it (a lost TurnEnd
	// misreports the outcome as canceled, or as no end at all). A keeping-up
	// consumer has drained the buffer by terminal time, so the fast path lands
	// it; only a backed-up buffer falls through to the escape below.
	select {
	case st.events <- stamped:
		return true
	default:
	}
	// Buffer full: block until the consumer drains, or bail when the turn ctx is
	// canceled so a producer never wedges on an abandoned channel.
	select {
	case st.events <- stamped:
		return true
	case <-st.ctx.Done():
		return false
	}
}

func (st *turnState) closeEvents() {
	st.eventMu.Lock()
	defer st.eventMu.Unlock()
	if st.eventsClosed {
		return
	}
	st.eventsClosed = true
	close(st.events)
}
