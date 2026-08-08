package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// SteeringSink persists queued steering after the current Run finishes.
type SteeringSink interface {
	AppendUserMessage(ctx context.Context, sessionID string, message corechat.Message) error
}

type steeringMessage struct {
	content []transcript.ContentBlock
	message corechat.Message
}

// InjectSteering validates and queues input on the active turn's pending
// steering buffer.
// The tool loop drains that buffer before each continuation round
// ([controller.steerSource]), so a message sent while the model is
// mid-loop reaches the CURRENT turn. Whatever arrives after the last round has
// no round left to drain it and falls back to next-turn semantics
// ([controller.flushSteering] writes it to the chat history store, where
// the next StartTurn's history middleware picks it up). Both paths share the one
// mutex-guarded queue, so a message is handled exactly once.
//
// Returns [ErrTurnNotFound] when the turn has already ended (its runTurn deleted
// itself from the map on exit). Invalid or empty content is rejected before the
// turn lookup so malformed input never depends on resource state.
func (s *controller) InjectSteering(_ context.Context, handle Handle, input []transcript.ContentBlock) error {
	message, err := runs.MaterializeUserMessage(input)
	if err != nil {
		return err
	}
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	// Rejects with ErrTurnNotFound if the turn has closed its steering queue
	// (terminating) — same signal as a vanished turn, so SteerRun maps both to
	// run_not_found and the client retries as a fresh send.
	return state.appendSteering(steeringMessage{
		content: append([]transcript.ContentBlock(nil), input...),
		message: message,
	})
}

// steerSource builds the SteerSource the engine's tool loop drains before each
// continuation round (mid-run steering): it pops the pending queue, surfaces
// each message as a [SteerMessage] event (so the steered turn shows on the
// timeline + lands in the durable transcript), and returns them as user
// messages for injection into the loop. Anything that arrives after the last
// round drains to nothing here and is picked up by the next-turn
// [controller.flushSteering] fallback — same mutex-guarded queue, never
// double-handled. The closure runs on the engine's turn goroutine, so emit is
// sequential with the turn's other events.
func (s *controller) steerSource(st *turnState) agentexec.SteerSource {
	return func() []corechat.Message {
		queue := st.drainSteering()
		if len(queue) == 0 {
			return nil
		}
		out := make([]corechat.Message, len(queue))
		for i, queued := range queue {
			s.emitRootEvent(st, runs.SteerMessage{
				Content: append([]transcript.ContentBlock(nil), queued.content...),
			})
			out[i] = queued.message.Clone()
		}
		return out
	}
}

// flushSteering publishes and persists steering that missed the final live
// round. Publishing gives the accepted input its durable transcript Item and
// reconciles the client's optimistic bubble; persisting lets the next turn's
// model consume the same message. A message drained by steerSource is absent
// from this closed queue, so the two paths cannot publish it twice.
//
// No-op when there's no session or no queued steering.
// Failures are recorded on the turn span but never mutate an already-decided
// execution outcome.
func (s *controller) flushSteering(ctx context.Context, st *turnState, sessionID string) {
	queue := st.closeAndDrainSteering()
	if sessionID == "" || len(queue) == 0 {
		return
	}
	for _, queued := range queue {
		if !s.emitRootEvent(st, runs.SteerMessage{
			Content: append([]transcript.ContentBlock(nil), queued.content...),
		}) {
			recordMaintenanceError(st, errors.New("steering transcript publication failed"))
		}
		if s.steering == nil {
			recordMaintenanceError(st, errors.New("steering inject failed: no steering sink configured"))
			return
		}
		if err := s.steering.AppendUserMessage(ctx, sessionID, queued.message); err != nil {
			recordMaintenanceError(st, err)
			return
		}
	}
}
