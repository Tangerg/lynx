package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// PrepareWaitingSubtreeCancellation freezes one exact waiting Interaction tree
// until the returned Application capability is applied or discarded.
func (i *InteractionExecutor) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	request runs.WaitingSubtreeCancellationRequest,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	if err := request.Validate(); err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	ref := runs.ExecutorRef{
		SessionID:  request.Continuation.SessionID,
		ExecutorID: request.Continuation.ExecutorID,
	}
	session, err := i.session(ref)
	if errors.Is(err, runs.ErrExecutorNotLive) {
		if restoreErr := i.restoreWaitingTree(
			ctx,
			ref,
			request.Continuation,
			interactionBoundaryWaiting,
		); restoreErr != nil {
			return runs.PreparedWaitingSubtreeCancellation{}, restoreErr
		}
		session, err = i.session(ref)
	}
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	return session.prepareWaitingSubtreeCancellation(
		ctx,
		request.Continuation.Checkpoint,
		request.TargetMemberID,
		request.Reason,
	)
}

func (i *interactionSession) prepareWaitingSubtreeCancellation(
	ctx context.Context,
	expectedCheckpoint runs.ExecutorCheckpoint,
	memberID string,
	reason string,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	if ctx == nil {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New(
			"agentexec: waiting subtree preparation context is required",
		)
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return runs.PreparedWaitingSubtreeCancellation{}, errors.New(
			"agentexec: waiting subtree preparation requires a deadline",
		)
	}
	targetID, err := agent.ParseProcessID(memberID)
	if err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"agentexec: parse waiting subtree member: %w",
			err,
		)
	}
	i.state.mu.Lock()
	if i.state.finished || i.state.process == nil {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, runs.ErrExecutorNotLive
	}
	switch {
	case i.state.boundary != interactionBoundaryWaiting:
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree is crossing another execution boundary",
			runs.ErrExecutionClaimed,
		)
	case i.state.observerWasAttached:
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree has an active observer",
			runs.ErrExecutionClaimed,
		)
	case !isInteractionWaitingBoundary(i.state.process.Status()):
		status := i.state.process.Status()
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction root is %s",
			runs.ErrExecutionClaimed,
			status,
		)
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, expectedCheckpoint) {
		i.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: live Interaction checkpoint differs from the waiting subtree request",
			runs.ErrInvalidExecutorCheckpoint,
		)
	}
	rootID := i.state.process.Relation().RootID()
	preparedSignal := make(chan struct{})
	i.state.boundary = interactionBoundarySubtreePreparing
	i.state.subtreePrepared = preparedSignal
	i.state.mu.Unlock()

	frameworkChange, err := i.engine.PrepareWaitingSubtreeCancellation(
		ctx,
		rootID,
		targetID,
		reason,
	)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"agentexec: prepare waiting Interaction subtree: %w",
			err,
		)
	}
	discard := true
	defer func() {
		if discard {
			_ = frameworkChange.Discard()
		}
	}()
	resultingTree := frameworkChange.ResultingSnapshot()
	checkpoint, err := i.executorCheckpoint(resultingTree)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	interruptions, err := i.pendingInterruptions(resultingTree)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	canceled := frameworkChange.CanceledProcessIDs()
	paused := frameworkChange.PausedProcessIDs()
	canceledMembers, err := i.executorMemberIDs(canceled)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	pausedMembers, err := i.executorMemberIDs(paused)
	if err != nil {
		i.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	change := &interactionWaitingSubtreeChange{
		session: i, prepared: frameworkChange, checkpoint: checkpoint.Clone(),
		canceled: slices.Clone(canceled), paused: slices.Clone(paused),
	}
	if err := i.completeSubtreePreparation(preparedSignal, change); err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	change.armExpiration(ctx)
	discard = false
	return runs.PreparedWaitingSubtreeCancellation{
		CanceledMemberIDs: canceledMembers, PausedMemberIDs: pausedMembers,
		PendingInterruptions: interruptions, Checkpoint: checkpoint, Change: change,
	}, nil
}

func (i *interactionSession) executorMemberIDs(
	processIDs []agent.ProcessID,
) ([]string, error) {
	members := make([]string, len(processIDs))
	for index, processID := range processIDs {
		member, found := i.executorMemberByProcessID(processID)
		if !found || member.MemberID != processID.String() {
			return nil, fmt.Errorf(
				"agentexec: Interaction Process %s has no exact product member",
				processID,
			)
		}
		members[index] = member.MemberID
	}
	return members, nil
}

func (i *interactionSession) failSubtreePreparation(preparedSignal chan struct{}) {
	i.state.mu.Lock()
	if i.state.boundary == interactionBoundarySubtreePreparing &&
		i.state.subtreePrepared == preparedSignal {
		i.state.boundary = interactionBoundaryWaiting
		i.state.subtreePrepared = nil
		close(preparedSignal)
	}
	i.state.mu.Unlock()
}

func (i *interactionSession) completeSubtreePreparation(
	preparedSignal chan struct{},
	change *interactionWaitingSubtreeChange,
) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.boundary != interactionBoundarySubtreePreparing ||
		i.state.subtreePrepared != preparedSignal || i.state.subtreeChange != nil {
		if i.state.subtreePrepared == preparedSignal {
			i.state.subtreePrepared = nil
			close(preparedSignal)
		}
		return runs.ErrExecutorNotLive
	}
	i.state.boundary = interactionBoundarySubtreePrepared
	i.state.subtreeChange = change
	i.state.subtreePrepared = nil
	close(preparedSignal)
	return nil
}
