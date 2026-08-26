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
// then durably opens the continuation Segment. That opening accepts the command;
// semantic answer submission continues behind the Run lifecycle supervisor.
func (c *Coordinator) Resume(ctx context.Context, cmd ResumeCommand) (result StartResult, err error) {
	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return StartResult{}, err
	}
	if !found {
		return StartResult{}, ErrInterruptNotOpen
	}
	if validateErr := pending.Validate(); validateErr != nil {
		return StartResult{}, fmt.Errorf("runs: invalid pending interrupt set: %w", validateErr)
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
	runAdmission, ok := c.admission.AcquireRun(pending.SessionID, sess.Workspace().Path())
	if !ok {
		return StartResult{}, fmt.Errorf("%w: session %q or working tree %q has a run or mutation in flight", ErrSessionBusy, pending.SessionID, sess.Workspace().Path())
	}
	defer runAdmission.Release()
	parkedRuns, err := c.runs.Tree(ctx, pending.RootRunID)
	if err != nil {
		return StartResult{}, err
	}
	if validatePendingRunTreeErr := validatePendingRunTree(pending, parkedRuns); validatePendingRunTreeErr != nil {
		return StartResult{}, validatePendingRunTreeErr
	}

	claim := ResumeClaimCommit{
		CommitID: newRunCommitID(), Expected: pending, Answers: answers, ClaimedAt: c.publications.nowUTC(),
	}
	claimed, err := c.resumeClaims.ClaimResume(ctx, claim)
	if err != nil {
		return StartResult{}, err
	}
	attempt := c.ownClaimedResume(pending)
	defer func() {
		err = attempt.fail(ctx, err)
		if err != nil {
			result = StartResult{}
		}
	}()
	if validateClaimedResumeErr := validateClaimedResume(claimed, pending, answers, sess); validateClaimedResumeErr != nil {
		return StartResult{}, validateClaimedResumeErr
	}
	rootContinuation, ok := pending.RootContinuation()
	if !ok {
		return StartResult{}, errors.New("runs: pending interrupt set has no root continuation")
	}
	ref, err := c.continuation.StageContinuation(ctx, WaitingContinuation{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: waitingMembersFromPending(pending),
		Checkpoint:               claimed.Checkpoint,
		Capabilities:             pending.Capabilities,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	})
	if err != nil {
		return StartResult{}, err
	}
	attempt.ownStagedExecution(&c.segments, ref)
	if validateForErr := attempt.staged.validateFor(pending.SessionID); validateForErr != nil {
		return StartResult{}, validateForErr
	}
	segmentID := c.newSegmentID()
	createdAt := rootContinuation.RunCreatedAt
	pendingCopy := pending
	continuation, err := treeContinuationFromPending(pendingCopy)
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare tree continuation: %w", err)
	}
	approvalResolutions, err := claim.ToolApprovalResolutions()
	if err != nil {
		return StartResult{}, fmt.Errorf("runs: prepare Tool approval continuation: %w", err)
	}
	if bindToolApprovalResolutionsErr := continuation.bindToolApprovalResolutions(approvalResolutions); bindToolApprovalResolutionsErr != nil {
		return StartResult{}, fmt.Errorf("runs: bind Tool approval continuation: %w", bindToolApprovalResolutionsErr)
	}
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:             cmd.RunID,
		SegmentID:         segmentID,
		SessionID:         pending.SessionID,
		CWD:               sess.Workspace().Path(),
		ExecutorID:        ref.ExecutorID,
		ModelSelection:    rootContinuation.ModelSelection,
		GoalIncarnationID: pending.GoalIncarnationID,
		CreatedAt:         createdAt,
		Input:             cmd.Input,
		Continuation:      continuation,
		admission:         &runAdmission,
		DetachActivation:  true,
		BeginExecution: func(beginCtx context.Context) error {
			return c.continuation.BeginContinuation(
				beginCtx, ref, claimed.Answers, pending.Capabilities.InterruptKinds,
			)
		},
	})
	if err != nil {
		return StartResult{}, err
	}
	// A successful opening transferred the staged tree to the Segment lifecycle.
	// On failure, the attempt still owns it and compensates durable state first.
	attempt.accept()
	// The continuation is durably accepted, which consumed the whole open set: the
	// run is running again and nothing in this session is waiting on a person.
	for _, continuation := range pending.Continuations {
		if continuation.RunID == pending.RootRunID {
			continue
		}
		c.publications.publishRunMoved(pending.SessionID, continuation.RunID)
	}
	c.publications.publishWaitingMoved(pending.SessionID, pending.RootRunID)
	result = StartResult{RunID: cmd.RunID, SegmentID: segmentID, SessionID: pending.SessionID, Events: events}
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
