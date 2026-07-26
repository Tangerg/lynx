package turn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/chatclient"
)

// Rehydrate rebuilds a parked turn from a persisted process snapshot without
// delivering the user's decision. It rebuilds process-local state under the
// persisted turn handle and leaves the restored process parked so the run
// coordinator can first establish the event owner and atomically accept the
// continuation; [Resume] delivers the decision only after those gates succeed.
func (s *memoryDispatcher) Rehydrate(ctx context.Context, request runs.RehydrateTurn) (TurnHandle, error) {
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
	state := newRestoringTurnState(ctx, handle)
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
	if request.ModelSelection.Configured() && s.resolver != nil {
		c, err := s.resolver.ResolveClient(state.ctx, request.ModelSelection)
		if err != nil {
			state.cancel()
			return TurnHandle{}, err
		}
		client = c
		state.model = request.ModelSelection.Model()
	} else {
		state.model = "default"
	}
	state.modelSelection = request.ModelSelection
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{dispatcher: s, st: state}
	subagents := newSubagentLifecycle(state.handle.SessionID, state.cwd, state.hooks)
	var eventListener core.Extension
	if subagents != nil {
		eventListener = subagents.listener(handle.TurnID)
	}
	if !s.register(state) {
		state.cancel()
		state.span.End()
		return TurnHandle{}, ErrDispatcherClosed
	}

	process, err := s.engine.RestoreTurn(state.ctx, request.ProcessID, agentexec.RestoreTurnRequest{
		SessionID:     request.SessionID,
		Observer:      observer,
		EventListener: eventListener,
		ChatClient:    client,
	})
	if err != nil {
		return TurnHandle{}, errors.Join(
			err,
			s.finishExecutionError(state, problemFromError(err), err),
		)
	}
	if process == nil {
		err := errors.New("turn: engine returned a nil restored process")
		return TurnHandle{}, errors.Join(
			err,
			s.finishExecutionError(state, internalRunProblem(), err),
		)
	}
	if subagents != nil {
		if err := subagents.confirmRoot(process.ID()); err != nil {
			state.setRestoredProcess(process)
			state.cancel()
			return TurnHandle{}, errors.Join(
				err,
				cancelTurnProcess(state.ctx, process),
				s.finishExecutionError(state, internalRunProblem(), err),
			)
		}
	}
	live := state.setRestoredProcess(process)
	if s.isClosed() {
		// BeginShutdown captured this registered state before RestoreTurn crossed
		// the process-publication boundary. The shutdown owner is waiting on the
		// lifecycle notification and now owns teardown.
		return TurnHandle{}, ErrDispatcherClosed
	}
	if !live {
		// A normal Cancel (not dispatcher shutdown) won while RestoreTurn was in
		// flight. Process publication makes cancellation actionable, so this
		// publisher completes the ownership handoff synchronously.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), processDiscardTimeout)
		cancelErr := s.Cancel(cleanupCtx, handle)
		cancel()
		if errors.Is(cancelErr, ErrTurnNotFound) {
			cancelErr = nil
		}
		return TurnHandle{}, errors.Join(ErrParkClaimed, cancelErr)
	}

	return handle, nil
}
