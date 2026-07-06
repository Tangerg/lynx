package runtime

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// StartTurn launches one agent turn through the runtime facade. An explicit
// model selection is recorded on the session before the turn is dispatched, so
// every caller gets the same session-model invariant.
func (r *Runtime) StartTurn(ctx context.Context, req turn.StartTurnRequest) (turn.TurnHandle, error) {
	if req.Model != "" {
		if err := r.session.SetModel(ctx, req.SessionID, req.Model); err != nil {
			return turn.TurnHandle{}, err
		}
	}
	return r.turnSvc.StartTurn(ctx, req)
}

// TurnEvents subscribes to the event stream for a live turn.
func (r *Runtime) TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error) {
	return r.turnSvc.Events(ctx, handle)
}

// InjectTurnSteering queues an in-flight steering message for a live turn.
func (r *Runtime) InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error {
	return r.turnSvc.InjectSteering(ctx, handle, message)
}

// ResumeTurn answers a parked HITL turn.
func (r *Runtime) ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution) error {
	return r.turnSvc.Resume(ctx, handle, resolution)
}

// RehydrateTurn rebuilds and resumes a parked turn after process-local state was lost.
func (r *Runtime) RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return r.turnSvc.Rehydrate(ctx, req)
}

// CancelTurn stops a live or parked turn.
func (r *Runtime) CancelTurn(ctx context.Context, handle turn.TurnHandle) error {
	return r.turnSvc.Cancel(ctx, handle)
}

// TurnProcessID returns the persisted agent-process id backing a parked turn.
func (r *Runtime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return r.turnSvc.ProcessID(ctx, handle)
}

// SetTurnInterruptKinds records the HITL interrupt kinds the connected client can answer.
func (r *Runtime) SetTurnInterruptKinds(kinds []string) {
	r.turnSvc.SetInterruptKinds(kinds)
}
