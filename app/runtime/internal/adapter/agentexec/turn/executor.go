package turn

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// executorControl is the turn control slice the application run adapter
// needs. It lives at the consumer because the concrete controller owns no
// reusable abstraction boundary.
type executorControl interface {
	Events(ctx context.Context, handle Handle) (iter.Seq[runs.ExecutorEvent], error)
	InjectSteering(ctx context.Context, handle Handle, content []transcript.ContentBlock) error
	PrepareTurn(ctx context.Context, start runs.StartExecution) (Handle, error)
	ActivateTurn(ctx context.Context, handle Handle) error
	Resume(ctx context.Context, handle Handle, answers []agentexec.SuspensionAnswer, interruptKinds []interrupt.Kind) error
	ProcessID(ctx context.Context, handle Handle) (string, error)
	Rehydrate(ctx context.Context, execution runs.RehydrateExecution) (Handle, error)
	Cancel(ctx context.Context, handle Handle) error
	CancelSubtree(ctx context.Context, handle Handle, processID string) error
}

type waitingSubtreeControl interface {
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		handle Handle,
		processID string,
	) (runs.PreparedWaitingSubtreeCancellation, error)
}

// Executor adapts the internal turn controller to the Run execution ports. It
// maps adapter-local Handles to [runs.ExecutorRef] and observations to the
// application-owned event family.
type Executor struct {
	controller executorControl
}

// NewExecutor returns an Executor over the turn controller.
func NewExecutor(controller executorControl) *Executor {
	return &Executor{controller: controller}
}

// TurnEvents subscribes to a live turn addressed by its durable application
// identity; each rich turn event is translated into the engine-neutral event
// contract.
func (e *Executor) Events(ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	seq, err := e.controller.Events(ctx, concreteHandle(ref))
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// CancelExecution stops a live or parked turn by durable identity.
func (e *Executor) CancelExecution(ctx context.Context, ref runs.ExecutorRef) error {
	return mapControlError(e.controller.Cancel(ctx, concreteHandle(ref)))
}

// CancelSubtree stops one descendant process tree without canceling its owning
// turn. The controller validates process ownership before calling Agent Runtime.
func (e *Executor) CancelSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	processID string,
) error {
	return mapControlError(e.controller.CancelSubtree(ctx, concreteHandle(ref), processID))
}

func (e *Executor) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	ref runs.ExecutorRef,
	processID string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	controller, ok := e.controller.(waitingSubtreeControl)
	if !ok {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New("turn executor: waiting subtree cancellation is unavailable")
	}
	prepared, err := controller.PrepareWaitingSubtreeCancellation(
		ctx,
		concreteHandle(ref),
		processID,
	)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, mapControlError(err)
	}
	return prepared, nil
}

// ValidateStart applies application-owned turn invariants plus the adapter's
// model-catalog modality check before a run resolves or creates a session.
func (e *Executor) ValidateStart(request runs.StartExecution) error {
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
func (e *Executor) PrepareStart(ctx context.Context, request runs.StartExecution) (runs.ExecutorRef, error) {
	handle, err := e.controller.PrepareTurn(ctx, request)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	return executorRef(handle), nil
}

// Activate crosses the fresh turn's model/tool side-effect boundary.
func (e *Executor) Activate(ctx context.Context, ref runs.ExecutorRef) error {
	return mapControlError(e.controller.ActivateTurn(ctx, concreteHandle(ref)))
}

// Prepare claims a process-local parked turn without delivering its decision.
func (e *Executor) Prepare(ctx context.Context, ref runs.ExecutorRef) (runs.ExecutorRef, error) {
	handle := concreteHandle(ref)
	if _, err := e.controller.ProcessID(ctx, handle); err != nil {
		return runs.ExecutorRef{}, mapControlError(err)
	}
	return executorRef(handle), nil
}

// Resume activates an already-attached continuation.
func (e *Executor) Resume(ctx context.Context, ref runs.ExecutorRef, answers []runs.SuspensionAnswer, interruptKinds []interrupt.Kind) error {
	executorAnswers := make([]agentexec.SuspensionAnswer, len(answers))
	for index, answer := range answers {
		executorAnswers[index] = agentexec.SuspensionAnswer{
			ProcessID:    answer.ProcessID,
			SuspensionID: answer.SuspensionID,
			Resolution:   answer.Resolution,
		}
	}
	return mapControlError(e.controller.Resume(ctx, concreteHandle(ref), executorAnswers, interruptKinds))
}

// Rehydrate rebuilds a parked turn from its durable executor checkpoint.
func (e *Executor) Rehydrate(ctx context.Context, request runs.RehydrateExecution) (runs.ExecutorRef, error) {
	handle, err := e.controller.Rehydrate(ctx, request)
	if err != nil {
		return runs.ExecutorRef{}, mapControlError(err)
	}
	return executorRef(handle), nil
}

// Steer injects structured user content into a live turn addressed by neutral
// identity.
func (e *Executor) Steer(ctx context.Context, ref runs.ExecutorRef, input []transcript.ContentBlock) error {
	return mapControlError(e.controller.InjectSteering(ctx, concreteHandle(ref), input))
}

func concreteHandle(ref runs.ExecutorRef) Handle {
	return Handle{SessionID: ref.SessionID, TurnID: ref.ExecutorID}
}

func executorRef(handle Handle) runs.ExecutorRef {
	return runs.ExecutorRef{SessionID: handle.SessionID, ExecutorID: handle.TurnID}
}

func mapControlError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, agentexec.ErrExecutorCheckpointLost):
		return fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
	case errors.Is(err, ErrParkClaimed):
		return fmt.Errorf("%w: %w", runs.ErrExecutionClaimed, err)
	case errors.Is(err, ErrTurnNotFound):
		return fmt.Errorf("%w: %w", runs.ErrExecutorNotLive, err)
	default:
		return err
	}
}
