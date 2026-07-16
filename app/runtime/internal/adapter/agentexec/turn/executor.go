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

// Executor adapts the turn [Dispatcher] to the application's run executor port
// (application/runs.SegmentExecutor): it drives, observes, and cancels the agent turn
// backing a run segment. The application holds the run lifecycle and drives
// execution through this port, so both the handle it hands back and the events it
// observes are normalized into the application-owned event family. Construct
// via [NewExecutor]; the composition root injects it into the run coordinator.
type Executor struct {
	dispatcher Dispatcher
}

// NewExecutor returns an Executor over the turn dispatcher.
func NewExecutor(dispatcher Dispatcher) *Executor {
	return &Executor{dispatcher: dispatcher}
}

// TurnEvents subscribes to a live turn's event stream. The opaque handle is
// asserted back to the [TurnHandle] the dispatcher minted; each rich turn event
// is translated into the engine-neutral application event contract.
func (e *Executor) TurnEvents(ctx context.Context, handle any) (iter.Seq[runs.EngineEvent], error) {
	h, ok := handle.(TurnHandle)
	if !ok {
		return nil, fmt.Errorf("turn: executor handle %T is not a turn handle", handle)
	}
	seq, err := e.dispatcher.Events(ctx, h)
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// CancelTurn stops a live or parked turn, asserting the opaque handle back to the
// dispatcher's [TurnHandle].
func (e *Executor) CancelTurn(ctx context.Context, handle any) error {
	h, ok := handle.(TurnHandle)
	if !ok {
		return fmt.Errorf("turn: executor handle %T is not a turn handle", handle)
	}
	return e.dispatcher.Cancel(ctx, h)
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

// Start launches a fresh executor turn and returns its neutral identity plus an
// opaque handle for the segment supervisor.
func (e *Executor) Start(ctx context.Context, request runs.StartTurn) (runs.Turn, error) {
	handle, err := e.dispatcher.StartTurn(ctx, StartTurnRequest{
		SessionID:      request.SessionID,
		Message:        request.Message,
		Media:          request.Media,
		Cwd:            request.Cwd,
		Provider:       request.Provider,
		Model:          request.Model,
		MaxBudget:      request.MaxBudget,
		MaxCostUSD:     request.MaxCostUSD,
		MaxSteps:       request.MaxSteps,
		Options:        request.Options,
		InterruptKinds: request.InterruptKinds,
	})
	if err != nil {
		return runs.Turn{}, err
	}
	return neutralTurn(handle), nil
}

// Prepare claims a process-local parked turn without delivering its decision.
func (e *Executor) Prepare(ctx context.Context, ref runs.TurnRef) (runs.Turn, error) {
	handle := concreteHandle(ref)
	if _, err := e.dispatcher.ProcessID(ctx, handle); err != nil {
		return runs.Turn{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Resume activates an already-attached continuation.
func (e *Executor) Resume(ctx context.Context, prepared runs.Turn, resolution interrupts.Resolution, interruptKinds []string) error {
	handle, err := recoverHandle(prepared.Handle)
	if err != nil {
		return err
	}
	return mapControlError(e.dispatcher.Resume(ctx, handle, resolution, interruptKinds))
}

// Rehydrate rebuilds a parked turn from its durable process snapshot.
func (e *Executor) Rehydrate(ctx context.Context, request runs.RehydrateTurn) (runs.Turn, error) {
	handle, err := e.dispatcher.Rehydrate(ctx, RehydrateRequest{
		SessionID: request.SessionID,
		TurnID:    request.TurnID,
		ProcessID: request.ProcessID,
		Provider:  request.Provider,
		Model:     request.Model,
		Cwd:       request.Cwd,
	})
	if err != nil {
		return runs.Turn{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Cancel tears down a live or parked turn addressed by neutral identity.
func (e *Executor) Cancel(ctx context.Context, ref runs.TurnRef) error {
	return e.dispatcher.Cancel(ctx, concreteHandle(ref))
}

// Steer injects a message into a live turn addressed by neutral identity.
func (e *Executor) Steer(ctx context.Context, ref runs.TurnRef, message string) error {
	return mapControlError(e.dispatcher.InjectSteering(ctx, concreteHandle(ref), message))
}

func concreteHandle(ref runs.TurnRef) TurnHandle {
	return TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
}

func neutralTurn(handle TurnHandle) runs.Turn {
	return runs.Turn{SessionID: handle.SessionID, TurnID: handle.TurnID, Handle: handle}
}

func recoverHandle(handle runs.Handle) (TurnHandle, error) {
	h, ok := handle.(TurnHandle)
	if !ok {
		return TurnHandle{}, fmt.Errorf("turn: executor handle %T is not a turn handle", handle)
	}
	return h, nil
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
