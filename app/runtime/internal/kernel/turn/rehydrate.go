package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// Rehydrate rebuilds a turn from a persisted process snapshot and resumes it —
// the cross-restart counterpart to [Resume]. It registers a fresh turn (new
// handle), restores + re-parks the agent process via [kernel.RestoreTurn] with
// a fresh observer + lifecycle listener, then delivers the decision and drives
// the continuation onto the new turn's event channel. The caller subscribes via
// [Events] on the returned handle.
func (s *inMemory) Rehydrate(ctx context.Context, req RehydrateRequest) (TurnHandle, error) {
	if req.ProcessID == "" {
		return TurnHandle{}, errors.New("turn: ProcessID is required")
	}
	handle := TurnHandle{SessionID: req.SessionID, TurnID: newTurnID()}
	state := newTurnState(ctx, handle)
	// Re-resolve the parked run's per-run client from the persisted
	// provider+model so the continuation runs against the SAME model (mirrors
	// the StartTurn path). No selection / no resolver / a provider since removed
	// → nil client = platform default, and the span records "default".
	var client *corechat.Client
	if req.Provider != "" && req.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(state.ctx, req.Provider, req.Model)
		if err != nil {
			state.cancel()
			return TurnHandle{}, err
		}
		client = c
		state.model = req.Model
	} else {
		state.model = "default"
	}
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{svc: s, st: state}
	state.lifecycle = &turnLifecycle{}

	proc, err := s.engine.RestoreTurn(state.ctx, req.ProcessID, kernel.RestoreTurnRequest{
		SessionID:     req.SessionID,
		Observer:      observer,
		EventListener: state.lifecycle.listener(handle.TurnID),
		ChatClient:    client,
	})
	if err != nil {
		state.cancel()
		return TurnHandle{}, err
	}
	state.lifecycle.setRoot(proc.ID())
	state.setProc(proc)

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	// The restored process is re-parked (RestoreTurn re-ticked it). Deliver the
	// decision and drive the continuation. On a resume error resumeAndDrive has
	// already torn the turn down (finishTurn), so there is no live turn for the
	// caller to subscribe to — return the error rather than a handle to a dead
	// turn (ResumeRun maps it to run_not_found instead of leaking ErrTurnNotFound
	// when its openSegment then can't find the turn). A nil error means the
	// continuation is driving and the caller subscribes via [Events].
	if err := s.resumeAndDrive(state, interrupts.Resolution{Approved: req.Approved}); err != nil {
		return TurnHandle{}, err
	}
	return handle, nil
}
