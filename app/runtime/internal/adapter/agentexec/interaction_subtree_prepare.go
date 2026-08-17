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
func (executor *InteractionExecutor) PrepareWaitingSubtreeCancellation(
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
	session, err := executor.session(ref)
	if errors.Is(err, runs.ErrExecutorNotLive) {
		if restoreErr := executor.restoreWaitingTree(
			ctx,
			ref,
			request.Continuation,
			interactionBoundaryWaiting,
		); restoreErr != nil {
			return runs.PreparedWaitingSubtreeCancellation{}, restoreErr
		}
		session, err = executor.session(ref)
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

func (session *interactionSession) prepareWaitingSubtreeCancellation(
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
	session.state.mu.Lock()
	if session.state.finished || session.state.process == nil {
		session.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, runs.ErrExecutorNotLive
	}
	switch {
	case session.state.boundary != interactionBoundaryWaiting:
		session.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree is crossing another execution boundary",
			runs.ErrExecutionClaimed,
		)
	case session.state.observerWasAttached:
		session.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction tree has an active observer",
			runs.ErrExecutionClaimed,
		)
	case !isInteractionWaitingBoundary(session.state.process.Status()):
		status := session.state.process.Status()
		session.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: Interaction root is %s",
			runs.ErrExecutionClaimed,
			status,
		)
	}
	if !executorCheckpointsEqual(session.state.waitingCheckpoint, expectedCheckpoint) {
		session.state.mu.Unlock()
		return runs.PreparedWaitingSubtreeCancellation{}, fmt.Errorf(
			"%w: live Interaction checkpoint differs from the waiting subtree request",
			runs.ErrInvalidExecutorCheckpoint,
		)
	}
	rootID := session.state.process.Relation().RootID()
	preparedSignal := make(chan struct{})
	session.state.boundary = interactionBoundarySubtreePreparing
	session.state.subtreePrepared = preparedSignal
	session.state.mu.Unlock()

	frameworkChange, err := session.engine.PrepareWaitingSubtreeCancellation(
		ctx,
		rootID,
		targetID,
		reason,
	)
	if err != nil {
		session.failSubtreePreparation(preparedSignal)
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
	checkpoint, err := session.executorCheckpoint(resultingTree)
	if err != nil {
		session.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	interruptions, err := session.pendingInterruptions(resultingTree)
	if err != nil {
		session.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	canceled := frameworkChange.CanceledProcessIDs()
	paused := frameworkChange.PausedProcessIDs()
	canceledMembers, err := session.executorMemberIDs(canceled)
	if err != nil {
		session.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	pausedMembers, err := session.executorMemberIDs(paused)
	if err != nil {
		session.failSubtreePreparation(preparedSignal)
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	change := &interactionWaitingSubtreeChange{
		session: session, prepared: frameworkChange, checkpoint: checkpoint.Clone(),
		canceled: slices.Clone(canceled), paused: slices.Clone(paused),
	}
	if err := session.completeSubtreePreparation(preparedSignal, change); err != nil {
		return runs.PreparedWaitingSubtreeCancellation{}, err
	}
	change.armExpiration(ctx)
	discard = false
	return runs.PreparedWaitingSubtreeCancellation{
		CanceledMemberIDs: canceledMembers, PausedMemberIDs: pausedMembers,
		PendingInterruptions: interruptions, Checkpoint: checkpoint, Change: change,
	}, nil
}

func (session *interactionSession) executorMemberIDs(
	processIDs []agent.ProcessID,
) ([]string, error) {
	members := make([]string, len(processIDs))
	for index, processID := range processIDs {
		member, found := session.executorMemberByProcessID(processID)
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

func (session *interactionSession) failSubtreePreparation(preparedSignal chan struct{}) {
	session.state.mu.Lock()
	if session.state.boundary == interactionBoundarySubtreePreparing &&
		session.state.subtreePrepared == preparedSignal {
		session.state.boundary = interactionBoundaryWaiting
		session.state.subtreePrepared = nil
		close(preparedSignal)
	}
	session.state.mu.Unlock()
}

func (session *interactionSession) completeSubtreePreparation(
	preparedSignal chan struct{},
	change *interactionWaitingSubtreeChange,
) error {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.finished || session.state.boundary != interactionBoundarySubtreePreparing ||
		session.state.subtreePrepared != preparedSignal || session.state.subtreeChange != nil {
		if session.state.subtreePrepared == preparedSignal {
			session.state.subtreePrepared = nil
			close(preparedSignal)
		}
		return runs.ErrExecutorNotLive
	}
	session.state.boundary = interactionBoundarySubtreePrepared
	session.state.subtreeChange = change
	session.state.subtreePrepared = nil
	close(preparedSignal)
	return nil
}
