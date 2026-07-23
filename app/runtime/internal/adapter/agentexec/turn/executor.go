package turn

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// executorDispatcher is the turn control slice the application run adapter
// needs. It lives at the consumer because the concrete dispatcher owns no
// reusable abstraction boundary.
type executorDispatcher interface {
	Events(context.Context, TurnHandle) (iter.Seq[runs.EngineEvent], error)
	InjectSteering(context.Context, TurnHandle, string) error
	PrepareTurn(context.Context, StartTurnRequest) (TurnHandle, error)
	ActivateTurn(context.Context, TurnHandle) error
	Resume(context.Context, TurnHandle, interrupts.Resolution, []string) error
	ProcessID(context.Context, TurnHandle) (string, error)
	Rehydrate(context.Context, RehydrateRequest) (TurnHandle, error)
	Cancel(context.Context, TurnHandle) error
}

// Executor adapts a turn dispatcher to the application's run executor port
// (application/runs.SegmentExecutor): it drives, observes, and cancels the agent turn
// backing a run segment. The application holds the run lifecycle and drives
// execution through this port, so both durable turn identity and observed
// events are normalized into the application-owned families. Construct
// via [NewExecutor]; the composition root injects it into the run coordinator.
type Executor struct {
	dispatcher executorDispatcher
}

// NewExecutor returns an Executor over the turn dispatcher.
func NewExecutor(dispatcher executorDispatcher) *Executor {
	return &Executor{dispatcher: dispatcher}
}

// TurnEvents subscribes to a live turn addressed by its durable application
// identity; each rich turn event is translated into the engine-neutral event
// contract.
func (e *Executor) TurnEvents(ctx context.Context, ref runs.TurnRef) (iter.Seq[runs.EngineEvent], error) {
	seq, err := e.dispatcher.Events(ctx, concreteHandle(ref))
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// CancelTurn stops a live or parked turn by durable identity.
func (e *Executor) CancelTurn(ctx context.Context, ref runs.TurnRef) error {
	return mapControlError(e.dispatcher.Cancel(ctx, concreteHandle(ref)))
}

// ValidateStart applies application-owned turn invariants plus the adapter's
// model-catalog modality check before a run resolves or creates a session.
func (e *Executor) ValidateStart(request runs.StartTurn) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if len(request.Media) > 0 && request.Provider != "" && request.Model != "" {
		if info, ok := catalog.Lookup(request.Provider, request.Model); ok && !info.Modalities.AcceptsInput(catalog.ModalityImage) {
			return fmt.Errorf("%w: model %q (provider %q) does not accept image input", runs.ErrUnsupportedMedia, request.Model, request.Provider)
		}
	}
	return nil
}

// PrepareStart creates a fresh executor turn without entering the model/tool
// engine. The application activates it only after durable run admission.
func (e *Executor) PrepareStart(ctx context.Context, request runs.StartTurn) (runs.TurnRef, error) {
	handle, err := e.dispatcher.PrepareTurn(ctx, StartTurnRequest{
		SessionID:      request.SessionID,
		Message:        request.Message,
		Media:          request.Media,
		Cwd:            request.Cwd,
		Isolated:       request.Isolated,
		Provider:       request.Provider,
		Model:          request.Model,
		MaxBudget:      request.MaxBudget,
		MaxCostUSD:     request.MaxCostUSD,
		MaxSteps:       request.MaxSteps,
		Options:        request.Options,
		InterruptKinds: request.InterruptKinds,
		GoalLeaseID:    request.GoalLeaseID,
	})
	if err != nil {
		return runs.TurnRef{}, err
	}
	return neutralTurn(handle), nil
}

// Activate crosses the fresh turn's model/tool side-effect boundary.
func (e *Executor) Activate(ctx context.Context, ref runs.TurnRef) error {
	return mapControlError(e.dispatcher.ActivateTurn(ctx, concreteHandle(ref)))
}

// Prepare claims a process-local parked turn without delivering its decision.
func (e *Executor) Prepare(ctx context.Context, ref runs.TurnRef) (runs.TurnRef, error) {
	handle := concreteHandle(ref)
	if _, err := e.dispatcher.ProcessID(ctx, handle); err != nil {
		return runs.TurnRef{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Resume activates an already-attached continuation.
func (e *Executor) Resume(ctx context.Context, ref runs.TurnRef, resolution interrupts.Resolution, interruptKinds []string) error {
	return mapControlError(e.dispatcher.Resume(ctx, concreteHandle(ref), resolution, interruptKinds))
}

// Rehydrate rebuilds a parked turn from its durable process snapshot.
func (e *Executor) Rehydrate(ctx context.Context, request runs.RehydrateTurn) (runs.TurnRef, error) {
	handle, err := e.dispatcher.Rehydrate(ctx, RehydrateRequest{
		SessionID: request.SessionID,
		TurnID:    request.TurnID,
		ProcessID: request.ProcessID,
		Provider:  request.Provider,
		Model:     request.Model,
		Cwd:       request.Cwd,
	})
	if err != nil {
		return runs.TurnRef{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Steer injects a message into a live turn addressed by neutral identity.
func (e *Executor) Steer(ctx context.Context, ref runs.TurnRef, message string) error {
	return mapControlError(e.dispatcher.InjectSteering(ctx, concreteHandle(ref), message))
}

func concreteHandle(ref runs.TurnRef) TurnHandle {
	return TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
}

func neutralTurn(handle TurnHandle) runs.TurnRef {
	return runs.TurnRef{SessionID: handle.SessionID, TurnID: handle.TurnID}
}

func mapControlError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, agentexec.ErrProcessSnapshotLost):
		return fmt.Errorf("%w: %w", runs.ErrTurnStateLost, err)
	case errors.Is(err, ErrParkClaimed):
		return fmt.Errorf("%w: %w", runs.ErrParkClaimed, err)
	case errors.Is(err, ErrTurnNotFound):
		return fmt.Errorf("%w: %w", runs.ErrTurnNotLive, err)
	default:
		return err
	}
}
