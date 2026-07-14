package turn

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/chatclient"
)

// Rehydrate rebuilds a parked turn from a persisted process snapshot without
// delivering the user's decision. It rebuilds process-local state under the
// persisted turn handle and leaves the restored process parked so the run
// coordinator can first establish the event owner and atomically accept the
// continuation; [Resume] delivers the decision only after those gates succeed.
func (s *inMemory) Rehydrate(ctx context.Context, req RehydrateRequest) (TurnHandle, error) {
	if req.ProcessID == "" {
		return TurnHandle{}, errors.New("turn: ProcessID is required")
	}
	if s.isClosed() {
		return TurnHandle{}, ErrDispatcherClosed
	}
	turnID := req.TurnID
	if turnID == "" {
		turnID = newTurnID()
	}
	handle := TurnHandle{SessionID: req.SessionID, TurnID: turnID}
	state := newTurnState(ctx, handle)
	// Re-resolve the parked run's per-run client from the persisted
	// provider+model so the continuation runs against the SAME model (mirrors
	// the StartTurn path). No selection / no resolver / a provider since removed
	// → nil client = platform default, and the span records "default".
	var client *chatclient.Client
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
	state.lifecycle = &turnLifecycle{sessionID: state.handle.SessionID}

	proc, err := s.engine.RestoreTurn(state.ctx, req.ProcessID, agentexec.RestoreTurnRequest{
		SessionID:     req.SessionID,
		Observer:      observer,
		EventListener: state.lifecycle.listener(handle.TurnID),
		ChatClient:    client,
	})
	if err != nil {
		state.cancel()
		state.span.End()
		return TurnHandle{}, err
	}
	state.lifecycle.setRoot(proc.ID())
	state.setProc(proc)
	if !state.parkIfLive() {
		_ = proc.Cancel()
		discardProcess(state.ctx, proc)
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	if !s.register(state) {
		_ = proc.Cancel()
		discardProcess(state.ctx, proc)
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	return handle, nil
}
