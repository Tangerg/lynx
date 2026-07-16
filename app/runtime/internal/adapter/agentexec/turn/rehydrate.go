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
func (s *memoryDispatcher) Rehydrate(ctx context.Context, request RehydrateRequest) (TurnHandle, error) {
	if request.ProcessID == "" {
		return TurnHandle{}, errors.New("turn: ProcessID is required")
	}
	if s.isClosed() {
		return TurnHandle{}, ErrDispatcherClosed
	}
	turnID := request.TurnID
	if turnID == "" {
		turnID = newTurnID()
	}
	handle := TurnHandle{SessionID: request.SessionID, TurnID: turnID}
	state := newTurnState(ctx, handle)
	// Re-resolve the parked run's per-run client from the persisted
	// provider+model so the continuation runs against the SAME model (mirrors
	// the StartTurn path). No selection / no resolver / a provider since removed
	// → nil client = engine default, and the span records "default".
	var client *chatclient.Client
	if request.Provider != "" && request.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(state.ctx, request.Provider, request.Model)
		if err != nil {
			state.cancel()
			return TurnHandle{}, err
		}
		client = c
		state.model = request.Model
	} else {
		state.model = "default"
	}
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{dispatcher: s, st: state}
	state.lifecycle = &turnLifecycle{sessionID: state.handle.SessionID}

	process, err := s.engine.RestoreTurn(state.ctx, request.ProcessID, agentexec.RestoreTurnRequest{
		SessionID:     request.SessionID,
		Observer:      observer,
		EventListener: state.lifecycle.listener(handle.TurnID),
		ChatClient:    client,
	})
	if err != nil {
		state.cancel()
		state.span.End()
		return TurnHandle{}, err
	}
	state.lifecycle.setRoot(process.ID())
	state.setProcess(process)
	if !state.parkIfLive() {
		_ = process.Cancel()
		discardProcess(state.ctx, process)
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	if !s.register(state) {
		_ = process.Cancel()
		discardProcess(state.ctx, process)
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	return handle, nil
}
