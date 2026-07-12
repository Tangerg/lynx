package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// StartTurn launches one agent turn through the runtime facade. An explicit
// model selection is recorded on the session before the turn is dispatched, so
// every caller gets the same session-model invariant.
func (r *Runtime) StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error) {
	if req.Model != "" {
		if err := r.sessions.SetModel(ctx, req.SessionID, req.Model); err != nil {
			return turn.TurnHandle{}, err
		}
	}
	return r.turns.StartTurn(ctx, req)
}

// InjectTurnSteering queues an in-flight steering message for a live turn.
func (r *Runtime) InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error {
	return r.turns.InjectSteering(ctx, handle, message)
}

// TurnProcessID returns the persisted agent-process id backing a parked turn.
func (r *Runtime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return r.turns.ProcessID(ctx, handle)
}
