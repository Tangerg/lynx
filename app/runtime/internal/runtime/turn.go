package runtime

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
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

// TurnEvents subscribes to the event stream for a live turn. It satisfies the
// application's engine-neutral [runs.Executor]: the handle arrives opaque and is
// asserted back to the turn handle the facade minted, and the rich turn events
// are forwarded as opaque [runs.EngineEvent] the delivery projector asserts back.
func (r *Runtime) TurnEvents(ctx context.Context, handle runs.Handle) (iter.Seq[runs.EngineEvent], error) {
	h, ok := handle.(turn.TurnHandle)
	if !ok {
		return nil, fmt.Errorf("runtime: executor handle %T is not a turn handle", handle)
	}
	seq, err := r.turns.Events(ctx, h)
	if err != nil {
		return nil, err
	}
	return func(yield func(runs.EngineEvent) bool) {
		for ev := range seq {
			if !yield(ev) {
				return
			}
		}
	}, nil
}

// InjectTurnSteering queues an in-flight steering message for a live turn.
func (r *Runtime) InjectTurnSteering(ctx context.Context, handle turn.TurnHandle, message string) error {
	return r.turns.InjectSteering(ctx, handle, message)
}

// ResumeTurn answers a parked HITL turn.
func (r *Runtime) ResumeTurn(ctx context.Context, handle turn.TurnHandle, resolution interrupts.Resolution, interruptKinds []string) error {
	return r.turns.Resume(ctx, handle, resolution, interruptKinds)
}

// RehydrateTurn rebuilds and resumes a parked turn after process-local state was lost.
func (r *Runtime) RehydrateTurn(ctx context.Context, req turn.RehydrateRequest) (turn.TurnHandle, error) {
	return r.turns.Rehydrate(ctx, req)
}

// CancelTurn stops a live or parked turn. It satisfies both the delivery turn
// use case (called with a concrete turn handle) and the application's opaque
// [runs.Executor]; the handle is asserted back to the facade's turn handle.
func (r *Runtime) CancelTurn(ctx context.Context, handle runs.Handle) error {
	h, ok := handle.(turn.TurnHandle)
	if !ok {
		return fmt.Errorf("runtime: executor handle %T is not a turn handle", handle)
	}
	return r.turns.Cancel(ctx, h)
}

// TurnProcessID returns the persisted agent-process id backing a parked turn.
func (r *Runtime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return r.turns.ProcessID(ctx, handle)
}
