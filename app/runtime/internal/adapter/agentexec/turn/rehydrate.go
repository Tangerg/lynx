package turn

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/chatclient"
)

// Rehydrate rebuilds a parked turn from a persisted executor checkpoint without
// delivering the user's decision. It rebuilds process-local state under the
// persisted turn handle and leaves the restored process parked so the run
// coordinator can first establish the event owner and atomically accept the
// continuation; [Resume] delivers the decision only after those gates succeed.
func (s *controller) Rehydrate(ctx context.Context, request runs.RehydrateExecution) (Handle, error) {
	if request.ProcessID == "" || strings.TrimSpace(request.ProcessID) != request.ProcessID {
		return Handle{}, errors.New("turn: process id must be non-empty without surrounding whitespace")
	}
	if err := runs.ValidateChildRunBindings(request.RootRunID, request.ChildRuns); err != nil {
		return Handle{}, fmt.Errorf("turn: restore child Run bindings: %w", err)
	}
	if s.isClosed() {
		return Handle{}, ErrClosed
	}
	if request.ModelSelection.Configured() && s.resolver == nil {
		return Handle{}, errors.New("turn: explicit model selection requires a client resolver")
	}
	turnID := request.ExecutorID
	if turnID == "" {
		turnID = newTurnID()
	}
	handle := Handle{SessionID: request.SessionID, TurnID: turnID}
	state := newRestoringTurnState(ctx, handle)
	state.cwd = request.Cwd
	if s.hooks != nil {
		var err error
		state.hooks, err = s.hooks.For(state.ctx, request.Cwd)
		if err != nil {
			state.cancel()
			return Handle{}, fmt.Errorf("turn: resolve lifecycle hooks while restoring process %q: %w", request.ProcessID, err)
		}
	}
	// Re-resolve the parked run's per-run client from the persisted
	// provider+model so the continuation runs against the same model (mirrors the
	// StartTurn path). An unset selection uses the engine default.
	var client *chatclient.Client
	if request.ModelSelection.Configured() {
		c, err := s.resolver.ResolveClient(state.ctx, request.ModelSelection)
		if err != nil {
			state.cancel()
			return Handle{}, err
		}
		if c == nil {
			state.cancel()
			return Handle{}, errors.New("turn: client resolver returned nil for an explicit model selection")
		}
		client = c
		state.model = request.ModelSelection.Model()
	} else {
		state.model = "default"
	}
	state.modelSelection = request.ModelSelection
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{
		controller:       s,
		st:               state,
		projectChildRuns: request.ChildRunAdmissionEnabled,
	}
	if err := observer.restoreChildRuns(request.ChildRuns); err != nil {
		state.cancel()
		state.span.End()
		return Handle{}, err
	}
	var admitChild agentexec.AdmitChildFunc
	if request.ChildRunAdmissionEnabled {
		admitChild = observer.admitChild
	}
	subagents := newSubagentLifecycle(
		state.handle.SessionID,
		state.cwd,
		state.hooks,
		observer.childRun,
		s.engine.SubagentProjection,
	)
	var eventListener core.Extension
	if subagents != nil {
		eventListener = subagents.listener(handle.TurnID)
	}
	if !s.register(state) {
		state.cancel()
		state.span.End()
		return Handle{}, ErrClosed
	}

	process, err := s.engine.RestoreTurn(state.ctx, request.ProcessID, agentexec.RestoreTurnRequest{
		SessionID:      request.SessionID,
		ModelSelection: request.ModelSelection,
		Cwd:            request.Cwd,
		WorkspaceCwd:   request.WorkspaceCwd,
		Isolated:       request.Isolated,
		GoalLeaseID:    request.GoalLeaseID,
		Limits:         request.Limits,
		Observer:       observer,
		EventListener:  eventListener,
		AdmitChild:     admitChild,
		ChatClient:     client,
	})
	if err != nil {
		return Handle{}, errors.Join(
			err,
			s.finishExecutionError(state, problemFromError(err), err),
		)
	}
	if process == nil {
		err := errors.New("turn: engine returned a nil restored process")
		return Handle{}, errors.Join(
			err,
			s.finishExecutionError(state, internalRunProblem(), err),
		)
	}
	if subagents != nil {
		if err := subagents.confirmRoot(process.ID()); err != nil {
			state.setRestoredProcess(process)
			state.cancel()
			return Handle{}, errors.Join(
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
		return Handle{}, ErrClosed
	}
	if !live {
		// A normal Cancel (not controller shutdown) won while RestoreTurn was in
		// flight. Process publication makes cancellation actionable, so this
		// publisher transfers the handoff to controller-owned work.
		if s.cleanupTasks.Start(ctx, func(ctx context.Context) {
			if cancelErr := s.Cancel(ctx, handle); cancelErr != nil && !errors.Is(cancelErr, ErrTurnNotFound) {
				recordTurnCleanupError(state, cancelErr)
			}
		}) {
			return Handle{}, ErrParkClaimed
		}
		// Shutdown already owns every registered turn and will observe the
		// process-publication lifecycle signal.
		return Handle{}, errors.Join(ErrParkClaimed, ErrClosed)
	}

	return handle, nil
}
