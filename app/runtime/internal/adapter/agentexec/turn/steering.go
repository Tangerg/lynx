package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// InjectSteering queues message onto the active turn's pending steering buffer.
// The runtime flushes the queue to the chat history store after the turn ends —
// every queued message becomes a user-role entry in the conversation history
// that the next turn's chat history middleware loads on the next StartTurn.
//
// This is "next-turn" semantics — not true mid-stream injection. Steering you
// send while the model is mid-tool-loop affects the next turn, not the current
// one. Documented limitation; doing real mid-stream injection would require
// intercepting between rounds of the chat tool loop.
//
// Returns [ErrTurnNotFound] when the turn has already ended (its runTurn deleted
// itself from the map on exit). Empty messages are silently dropped.
func (s *memoryDispatcher) InjectSteering(_ context.Context, handle TurnHandle, message string) error {
	if message == "" {
		return nil
	}
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	// Rejects with ErrTurnNotFound if the turn has closed its steering queue
	// (terminating) — same signal as a vanished turn, so SteerRun maps both to
	// run_not_found and the client retries as a fresh send.
	return state.appendSteering(message)
}

// steerSource builds the SteerSource the engine's tool loop drains before each
// continuation round (mid-run steering): it pops the pending queue, surfaces
// each message as a [SteerMessage] event (so the steered turn shows on the
// timeline + lands in the durable transcript), and returns them as user
// messages for injection into the loop. Anything that arrives after the last
// round drains to nothing here and is picked up by the next-turn
// [memoryDispatcher.flushSteering] fallback — same mutex-guarded queue, never
// double-handled. The closure runs on the engine's turn goroutine, so emit is
// sequential with the turn's other events.
func (s *memoryDispatcher) steerSource(st *turnState) agentexec.SteerSource {
	return func() []corechat.Message {
		queue := st.drainSteering()
		if len(queue) == 0 {
			return nil
		}
		out := make([]corechat.Message, len(queue))
		for i, m := range queue {
			s.emit(st, runs.SteerMessage{Text: m})
			out[i] = corechat.NewUserMessage(corechat.NewTextPart(m))
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
func (s *memoryDispatcher) flushSteering(ctx context.Context, st *turnState, sessionID string) {
	queue := st.closeAndDrainSteering()
	if sessionID == "" || len(queue) == 0 {
		return
	}
	for _, msg := range queue {
		if s.steering == nil {
			s.emit(st, runs.ErrorEvent{
				Message: "steering inject failed: no steering sink configured",
				Code:    runs.ErrorCodeSteering,
				Problem: internalRunProblem(),
			})
			return
		}
		if err := s.steering.InjectUser(ctx, sessionID, msg); err != nil {
			s.emit(st, runs.ErrorEvent{
				Message: "steering inject failed: " + err.Error(),
				Code:    runs.ErrorCodeSteering,
				Problem: internalRunProblem(),
			})
			return
		}
	}
}
