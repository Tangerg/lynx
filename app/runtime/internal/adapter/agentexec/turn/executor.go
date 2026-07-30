package turn

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// executorDispatcher is the turn control slice the application run adapter
// needs. It lives at the consumer because the concrete dispatcher owns no
// reusable abstraction boundary.
type executorDispatcher interface {
	Events(context.Context, TurnHandle) (iter.Seq[runs.ExecutorEvent], error)
	InjectSteering(context.Context, TurnHandle, []transcript.ContentBlock) error
	PrepareTurn(context.Context, runs.StartTurn) (TurnHandle, error)
	ActivateTurn(context.Context, TurnHandle) error
	Resume(context.Context, TurnHandle, []agentexec.SuspensionAnswer, []execution.InterruptKind) error
	ProcessID(context.Context, TurnHandle) (string, error)
	Rehydrate(context.Context, runs.RehydrateTurn) (TurnHandle, error)
	Cancel(context.Context, TurnHandle) error
	CancelSubtree(context.Context, TurnHandle, string) error
}

type waitingSubtreeDispatcher interface {
	PrepareWaitingSubtreeCancellation(
		context.Context,
		TurnHandle,
		string,
	) (runs.PreparedWaitingSubtreeCancellation, error)
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
func (e *Executor) TurnEvents(ctx context.Context, ref execution.TurnRef) (iter.Seq[runs.ExecutorEvent], error) {
	seq, err := e.dispatcher.Events(ctx, concreteHandle(ref))
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// CancelTurn stops a live or parked turn by durable identity.
func (e *Executor) CancelTurn(ctx context.Context, ref execution.TurnRef) error {
	return mapControlError(e.dispatcher.Cancel(ctx, concreteHandle(ref)))
}

// CancelSubtree stops one descendant process tree without canceling its owning
// turn. The dispatcher validates process ownership before calling Agent Runtime.
func (e *Executor) CancelSubtree(
	ctx context.Context,
	ref execution.TurnRef,
	processID string,
) error {
	return mapControlError(e.dispatcher.CancelSubtree(ctx, concreteHandle(ref), processID))
}

func (e *Executor) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	ref execution.TurnRef,
	processID string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	dispatcher, ok := e.dispatcher.(waitingSubtreeDispatcher)
	if !ok {
		return nil, errors.New("turn executor: waiting subtree cancellation is unavailable")
	}
	prepared, err := dispatcher.PrepareWaitingSubtreeCancellation(
		ctx,
		concreteHandle(ref),
		processID,
	)
	if err != nil {
		return nil, mapControlError(err)
	}
	return prepared, nil
}

// ValidateStart applies application-owned turn invariants plus the adapter's
// model-catalog modality check before a run resolves or creates a session.
func (e *Executor) ValidateStart(request runs.StartTurn) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if len(request.Media) > 0 && request.ModelSelection.Configured() {
		if info, ok := catalog.Lookup(request.ModelSelection.Provider(), request.ModelSelection.Model()); ok && !info.Modalities.AcceptsInput(catalog.ModalityImage) {
			return fmt.Errorf("%w: model %q (provider %q) does not accept image input", runs.ErrUnsupportedMedia, request.ModelSelection.Model(), request.ModelSelection.Provider())
		}
	}
	return nil
}

// PrepareStart creates a fresh executor turn without entering the model/tool
// engine. The application activates it only after durable run admission.
func (e *Executor) PrepareStart(ctx context.Context, request runs.StartTurn) (execution.TurnRef, error) {
	handle, err := e.dispatcher.PrepareTurn(ctx, request)
	if err != nil {
		return execution.TurnRef{}, err
	}
	return neutralTurn(handle), nil
}

// Activate crosses the fresh turn's model/tool side-effect boundary.
func (e *Executor) Activate(ctx context.Context, ref execution.TurnRef) error {
	return mapControlError(e.dispatcher.ActivateTurn(ctx, concreteHandle(ref)))
}

// Prepare claims a process-local parked turn without delivering its decision.
func (e *Executor) Prepare(ctx context.Context, ref execution.TurnRef) (execution.TurnRef, error) {
	handle := concreteHandle(ref)
	if _, err := e.dispatcher.ProcessID(ctx, handle); err != nil {
		return execution.TurnRef{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Resume activates an already-attached continuation.
func (e *Executor) Resume(ctx context.Context, ref execution.TurnRef, answers []interrupts.SuspensionAnswer, interruptKinds []execution.InterruptKind) error {
	executorAnswers := make([]agentexec.SuspensionAnswer, len(answers))
	for index, answer := range answers {
		executorAnswers[index] = agentexec.SuspensionAnswer{
			ProcessID:    answer.ProcessID,
			SuspensionID: answer.SuspensionID,
			Resolution:   answer.Resolution,
		}
	}
	return mapControlError(e.dispatcher.Resume(ctx, concreteHandle(ref), executorAnswers, interruptKinds))
}

// Rehydrate rebuilds a parked turn from its durable process snapshot.
func (e *Executor) Rehydrate(ctx context.Context, request runs.RehydrateTurn) (execution.TurnRef, error) {
	handle, err := e.dispatcher.Rehydrate(ctx, request)
	if err != nil {
		return execution.TurnRef{}, mapControlError(err)
	}
	return neutralTurn(handle), nil
}

// Steer injects structured user content into a live turn addressed by neutral
// identity.
func (e *Executor) Steer(ctx context.Context, ref execution.TurnRef, input []transcript.ContentBlock) error {
	return mapControlError(e.dispatcher.InjectSteering(ctx, concreteHandle(ref), input))
}

func concreteHandle(ref execution.TurnRef) TurnHandle {
	return TurnHandle{SessionID: ref.SessionID, TurnID: ref.TurnID}
}

func neutralTurn(handle TurnHandle) execution.TurnRef {
	return execution.TurnRef{SessionID: handle.SessionID, TurnID: handle.TurnID}
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
