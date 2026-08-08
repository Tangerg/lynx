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
	PrepareTurn(ctx context.Context, start runs.RootExecutionStart) (Handle, error)
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

// Observe subscribes to a live turn addressed by its durable application
// identity; each rich turn event is translated into the engine-neutral event
// contract.
func (e *Executor) Observe(ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	seq, err := e.controller.Events(ctx, concreteHandle(ref))
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// Release tears down the old executor's resources after the Application has
// decided the Run outcome or rejected an unadmitted stage.
func (e *Executor) Release(ctx context.Context, ref runs.ExecutorRef) error {
	return mapControlError(e.controller.Cancel(ctx, concreteHandle(ref)))
}

// CancelRunningSubtree stops one descendant process tree without canceling its owning
// turn. The controller validates process ownership before calling Agent Runtime.
func (e *Executor) CancelRunningSubtree(
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

// ValidateRootStart applies application-owned turn invariants plus the adapter's
// model-catalog modality check before a run resolves or creates a session.
func (e *Executor) ValidateRootStart(request runs.RootExecutionStart) error {
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

// StageRoot creates a fresh executor turn without entering the model/tool
// engine. The application activates it only after durable run admission.
func (e *Executor) StageRoot(ctx context.Context, request runs.RootExecutionStart) (runs.ExecutorRef, error) {
	handle, err := e.controller.PrepareTurn(ctx, request)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	return executorRef(handle), nil
}

// BeginRoot crosses the fresh turn's model/tool side-effect boundary.
func (e *Executor) BeginRoot(ctx context.Context, ref runs.ExecutorRef) error {
	return mapControlError(e.controller.ActivateTurn(ctx, concreteHandle(ref)))
}

// StageContinuation adapts the answer-claim-owned checkpoint to the old
// production executor without letting that executor reread persistence after
// the claim invalidated the durable recovery point.
func (e *Executor) StageContinuation(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if claimed, err := e.ClaimWaiting(ctx, ref); err == nil {
		return claimed, nil
	} else if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	checkpoint := continuation.Checkpoint.Clone()
	return e.RestoreWaiting(ctx, runs.RehydrateExecution{
		SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID,
		MemberID: checkpoint.RootMemberID, RootRunID: continuation.RootRunID,
		ChildRuns: continuation.ChildRuns, ModelSelection: checkpoint.ModelSelection,
		CWD: checkpoint.Scope.CWD, WorkspaceCWD: checkpoint.Scope.WorkspaceCWD,
		Isolated: checkpoint.Scope.Isolated, GoalLeaseID: checkpoint.Scope.GoalLeaseID,
		Limits:                   checkpoint.Limits,
		ChildRunAdmissionEnabled: continuation.ChildRunAdmissionEnabled,
		Checkpoint:               &checkpoint,
	})
}

func (e *Executor) BeginContinuation(
	ctx context.Context,
	ref runs.ExecutorRef,
	answers []runs.InterruptAnswer,
	allowedInterrupts []interrupt.Kind,
) error {
	return e.ResumeWaiting(ctx, ref, answers, allowedInterrupts)
}

// ClaimWaiting claims a process-local parked turn without delivering its decision.
func (e *Executor) ClaimWaiting(ctx context.Context, ref runs.ExecutorRef) (runs.ExecutorRef, error) {
	handle := concreteHandle(ref)
	if _, err := e.controller.ProcessID(ctx, handle); err != nil {
		return runs.ExecutorRef{}, mapControlError(err)
	}
	return executorRef(handle), nil
}

// ResumeWaiting activates an already-attached continuation.
func (e *Executor) ResumeWaiting(ctx context.Context, ref runs.ExecutorRef, answers []runs.InterruptAnswer, interruptKinds []interrupt.Kind) error {
	executorAnswers := make([]agentexec.SuspensionAnswer, len(answers))
	for index, answer := range answers {
		executorAnswers[index] = agentexec.SuspensionAnswer{
			ProcessID:    answer.MemberID,
			SuspensionID: answer.RequestID,
			Resolution:   answer.Resolution,
		}
	}
	return mapControlError(e.controller.Resume(ctx, concreteHandle(ref), executorAnswers, interruptKinds))
}

// RestoreWaiting rebuilds a parked turn from its durable executor checkpoint.
func (e *Executor) RestoreWaiting(ctx context.Context, request runs.RehydrateExecution) (runs.ExecutorRef, error) {
	handle, err := e.controller.Rehydrate(ctx, request)
	if err != nil {
		return runs.ExecutorRef{}, mapControlError(err)
	}
	return executorRef(handle), nil
}

// Steer injects structured user content into a live turn addressed by neutral
// identity.
func (e *Executor) SubmitSteer(ctx context.Context, ref runs.ExecutorRef, input []transcript.ContentBlock) error {
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
