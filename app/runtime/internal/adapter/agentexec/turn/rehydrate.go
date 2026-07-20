package turn

import (
	"context"
	"errors"
	"fmt"

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
	state.cwd = request.Cwd
	if s.hooks != nil {
		var err error
		state.hooks, err = s.hooks.For(state.ctx, request.Cwd)
		if err != nil {
			state.cancel()
			return TurnHandle{}, fmt.Errorf("turn: resolve lifecycle hooks while restoring process %q: %w", request.ProcessID, err)
		}
	}
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
		state.provider = request.Provider
	} else {
		state.model = "default"
	}
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{dispatcher: s, st: state}
	state.lifecycle = &turnLifecycle{
		sessionID: state.handle.SessionID,
		cwd:       state.cwd,
		hooks:     state.hooks,
	}

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
		return TurnHandle{}, rejectRestoredTurn(
			state,
			process,
			errors.New("turn: restored turn was canceled before registration"),
		)
	}

	if !s.register(state) {
		return TurnHandle{}, rejectRestoredTurn(state, process, ErrDispatcherClosed)
	}

	return handle, nil
}

// rejectRestoredTurn tears down a process restored during a dispatcher-close
// race. The close error and process cancellation failure are both preserved;
// snapshot discard remains terminal maintenance, but its failure is preserved
// alongside cancellation because this caller still owns a synchronous error
// boundary.
func rejectRestoredTurn(state *turnState, process agentexec.TurnProcess, cause error) error {
	cancelErr := cancelTurnProcess(state.ctx, process)
	recordTurnCleanupError(state, cancelErr)
	discardErr := discardProcess(state.ctx, process)
	recordTurnCleanupError(state, discardErr)
	state.cancel()
	state.span.End()
	return errors.Join(cause, cancelErr, discardErr)
}
