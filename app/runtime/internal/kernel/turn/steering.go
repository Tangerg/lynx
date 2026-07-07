package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// steerSource builds the SteerSource the engine's tool loop drains before each
// continuation round (mid-run steering): it pops the pending queue, surfaces
// each message as a [SteerMessage] event (so the steered turn shows on the
// timeline + lands in the durable transcript), and returns them as user
// messages for injection into the loop. Anything that arrives after the last
// round drains to nothing here and is picked up by the next-turn
// [inMemory.flushSteering] fallback — same mutex-guarded queue, never
// double-handled. The closure runs on the engine's turn goroutine, so emit is
// sequential with the turn's other events.
func (s *inMemory) steerSource(st *turnState) kernel.SteerSource {
	return func() []corechat.Message {
		queue := st.drainSteering()
		if len(queue) == 0 {
			return nil
		}
		out := make([]corechat.Message, len(queue))
		for i, m := range queue {
			s.emit(st, SteerMessage{Text: m})
			out[i] = corechat.NewUserMessage(m)
		}
		return out
	}
}

// flushSteering writes the turn's queued steering messages to the
// chat history store so the next turn picks them up as conversation
// history. No-op when there's no session or no queued steering.
// Errors surface through an ErrorEvent but don't abort the turn —
// dropping steering is preferable to wrecking an otherwise
// successful turn.
func (s *inMemory) flushSteering(ctx context.Context, st *turnState, sessionID string) {
	queue := st.closeAndDrainSteering()
	if sessionID == "" || len(queue) == 0 {
		return
	}
	for _, msg := range queue {
		if err := s.engine.InjectUserMessage(ctx, sessionID, msg); err != nil {
			s.emit(st, ErrorEvent{
				Message: "steering inject failed: " + err.Error(),
				Code:    "STEERING_ERROR",
			})
			return
		}
	}
}
