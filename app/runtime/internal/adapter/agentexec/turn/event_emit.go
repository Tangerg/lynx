package turn

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// emitRootEvent pushes a root-owned application event onto the turn's channel. A turn
// that failed before process creation has the reserved empty root source.
// Sends block when the consumer falls behind: the durable history (items.list)
// is built from this stream, so backpressure — the turn slowing to the
// consumer's persistence speed — is correct where dropping would silently
// corrupt persisted items (a lost MessageDelta truncates the item text; a lost
// TurnEnd misreports the outcome as canceled). The turn-lifetime ctx is the
// escape hatch: a canceled turn stops blocking producers even when no consumer
// is left to drain.
func (s *controller) emitRootEvent(st *turnState, payload runs.EngineEvent) bool {
	var source agentexec.ProcessRef
	if process := st.process(); process != nil {
		source = agentexec.ProcessRef{ID: process.ID()}
	}
	return s.emitProcessEvent(st, source, payload)
}

// emitProcessEvent preserves the concrete executor process that produced payload.
func (s *controller) emitProcessEvent(st *turnState, process agentexec.ProcessRef, payload runs.ExecutorPayload) bool {
	event := runs.ExecutorEvent{
		Source: runs.ExecutorSource{
			ProcessID: process.ID, ParentID: process.ParentID, SpawnCallID: process.SpawnCallID,
		},
		Payload: payload,
	}
	st.eventMu.Lock()
	defer st.eventMu.Unlock()
	if st.eventsClosed {
		return false
	}
	// Prefer enqueueing: when the buffer has room the event lands regardless of
	// whether the turn ctx was already canceled. This is what makes a canceled
	// turn's terminal event (TurnEnd) reaches a consumer still draining the
	// stream — Cancel cancels st.ctx *before* the
	// finishTurn / drive path emits the terminal, so a bare select would race
	// the terminal into the ctx.Done() escape and drop it (a lost TurnEnd
	// misreports the outcome as canceled, or as no end at all). A keeping-up
	// consumer has drained the buffer by terminal time, so the fast path lands
	// it; only a backed-up buffer falls through to the escape below.
	select {
	case st.events <- event:
		return true
	default:
	}
	// Buffer full: block until the consumer drains, or bail when the turn ctx is
	// canceled so a producer never wedges on an abandoned channel.
	select {
	case st.events <- event:
		return true
	case <-st.ctx.Done():
		return false
	}
}

func (st *turnState) closeEvents() {
	st.mu.Lock()
	st.eventsEnded = true
	st.mu.Unlock()

	st.eventMu.Lock()
	defer st.eventMu.Unlock()
	if st.eventsClosed {
		return
	}
	st.eventsClosed = true
	close(st.events)
}
