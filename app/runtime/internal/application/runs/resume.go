package runs

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// Resume validates one complete response set, atomically consumes the waiting
// hand-off and invalidates its checkpoint, stages the exact live/restored tree,
// then opens the continuation Segment before submitting semantic answers.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (StartResult, error) {
	if err := c.requireResumeDependencies(); err != nil {
		return StartResult{}, err
	}
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	if err := pending.Validate(); err != nil {
		return StartResult{}, fmt.Errorf("runs: invalid pending interrupt set: %w", err)
	}
	if gap := pending.Capabilities.MissingFrom(cmd.CallerCapabilities); !gap.IsEmpty() {
		return StartResult{}, &run.InsufficientCapabilitiesError{RunID: cmd.RunID, Missing: gap}
	}
	answers, err := resolveResumeResponses(pending, cmd.Responses)
	if err != nil {
		return StartResult{}, err
	}
	sess, err := c.sessionReader.Get(ctx, pending.SessionID)
	if err != nil {
		return StartResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(pending.SessionID, sess.CWD())
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, pending.SessionID, sess.CWD())
	}
	defer runAdmission.Release()
	parkedRuns, err := c.runs.Tree(ctx, pending.RootRunID)
	if err != nil {
		return StartResult{}, err
	}
	if err := validatePendingRunTree(pending, parkedRuns); err != nil {
		return StartResult{}, err
	}

	claimed, err := c.resumeClaims.ClaimResume(ctx, ResumeClaimCommit{
		Expected: pending, Answers: answers, ClaimedAt: c.now().UTC(),
	})
	if err != nil {
		return StartResult{}, err
	}
	if err := validateClaimedResume(claimed, pending, answers, sess); err != nil {
		return StartResult{}, c.failClaimedResume(ctx, pending, nil, err)
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, c.failClaimedResume(
			ctx, pending, nil, errors.New("runs: pending interrupt set has no root continuation"),
		)
	}
	ref, err := c.continuation.StageContinuation(ctx, WaitingContinuation{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: waitingMembersFromPending(pending),
		Checkpoint:               claimed.Checkpoint,
		Capabilities:             pending.Capabilities,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	})
	if err != nil {
		return StartResult{}, c.failClaimedResume(ctx, pending, nil, err)
	}
	if err := ref.ValidateFor(pending.SessionID); err != nil {
		return StartResult{}, c.failClaimedResume(ctx, pending, &ref, err)
	}
	segmentID := c.newSegmentID()
	createdAt := rootContinuation.RunCreatedAt
	pendingCopy := pending
	continuation, err := treeContinuationFromPending(pendingCopy)
	if err != nil {
		return StartResult{}, c.failClaimedResume(
			ctx, pending, &ref, fmt.Errorf("runs: prepare tree continuation: %w", err),
		)
	}
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:          cmd.RunID,
		SegmentID:      segmentID,
		SessionID:      pending.SessionID,
		CWD:            sess.CWD(),
		ExecutorID:     ref.ExecutorID,
		ModelSelection: rootContinuation.ModelSelection,
		GoalLeaseID:    pending.GoalLeaseID,
		CreatedAt:      createdAt,
		Input:          cmd.Input,
		Continuation:   continuation,
		admission:      &runAdmission,
		BeginExecution: func(beginCtx context.Context) error {
			return c.continuation.BeginContinuation(
				beginCtx, ref, claimed.Answers, pending.Capabilities.InterruptKinds,
			)
		},
	})
	if err != nil {
		return StartResult{}, c.failClaimedResume(ctx, pending, &ref, err)
	}
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publishRunMoved(pending.SessionID, continuation.RunID)
	}
	c.publishWaitingMoved(pending.SessionID, pending.RootRunID)
	result := StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}
	if len(cmd.Input) > 0 {
		// Named only when there is an item to name: the id is derived from the segment
		// the same way a fresh run derives it, so the client reconciles its optimistic
		// bubble by id rather than by content.
		result.UserItemID = userMessageItemID(segmentID)
	}
	return result, nil
}

func validateClaimedResume(
	claimed ClaimedResume,
	expected Pending,
	expectedAnswers []InterruptAnswer,
	sess session.Session,
) error {
	if !reflect.DeepEqual(claimed.Pending, expected) {
		return errors.New("runs: claimed waiting hand-off differs from the accepted Pending value")
	}
	if !reflect.DeepEqual(claimed.Answers, expectedAnswers) {
		return errors.New("runs: claimed answers differ from the accepted answer set")
	}
	if err := claimed.Checkpoint.Validate(); err != nil {
		return err
	}
	root, ok := expected.RootContinuation()
	if !ok {
		return errors.New("runs: claimed continuation has no root")
	}
	if err := claimed.Checkpoint.ValidateOwnership(root.MemberID, expected.SessionID); err != nil {
		return err
	}
	return validateCheckpointSessionScope(claimed.Checkpoint, sess)
}

func (c *Coordinator) failClaimedResume(
	ctx context.Context,
	pending Pending,
	ref *ExecutorRef,
	cause error,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if cleanupErr := c.terminations.ApplyRunLost(
		cleanupCtx,
		pending.SessionID,
		pending.RootRunID,
		c.now().UTC(),
	); cleanupErr != nil {
		return errors.Join(
			cause,
			fmt.Errorf("runs: recover claimed resume %q as lost: %w", pending.RootRunID, cleanupErr),
		)
	}
	result := fmt.Errorf("%w: %w", ErrRunNotFound, cause)
	if ref != nil {
		if releaseErr := c.releases.Release(cleanupCtx, *ref); releaseErr != nil {
			result = errors.Join(
				result,
				fmt.Errorf("runs: release lost continuation %q: %w", ref.ExecutorID, releaseErr),
			)
		}
	}
	return result
}

func waitingMembersFromPending(pending Pending) []WaitingMember {
	members := make([]WaitingMember, 0, len(pending.Continuations))
	for _, continuation := range pending.Continuations {
		parentRunID := ""
		if continuation.Lineage.IsChild() {
			parentRunID = continuation.Lineage.ParentRunID
		}
		members = append(members, WaitingMember{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			ParentRunID: parentRunID, SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
			ModelSelection: continuation.ModelSelection, Metrics: continuation.Metrics,
		})
	}
	return members
}
