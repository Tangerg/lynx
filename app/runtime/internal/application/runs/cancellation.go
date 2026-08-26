package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// Cancel handles both live and parked runs under the same run/session admission
// rules and returns the exact terminal Run committed by the winning write-set.
// The durable abandon write-set is authoritative and commits before a parked
// executor is torn down. Cleanup errors are returned unless the executor already
// disappeared, which is the idempotent completion race.
func (c *Coordinator) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	var err error
	cmd, err = cmd.normalizeReason()
	if err != nil {
		return CancelResult{}, err
	}
	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	if plan.target.run.Lineage().IsChild() {
		switch plan.treeState {
		case rundomain.Running:
			if !live || entry.owner == nil {
				return CancelResult{}, fmt.Errorf(
					"runs: running child Run %q has no live root owner",
					cmd.RunID,
				)
			}
			return c.cancelLiveChild(ctx, cmd, plan, entry.owner)
		case rundomain.Waiting:
			return c.cancelWaitingChild(ctx, cmd, plan)
		default:
			return CancelResult{}, fmt.Errorf(
				"runs: child Run %q belongs to a tree in state %s",
				cmd.RunID,
				plan.treeState,
			)
		}
	}
	if !live {
		return c.cancelWithoutLiveSegment(ctx, cmd, plan.root.run)
	}
	if entry.owner == nil {
		return CancelResult{}, fmt.Errorf(
			"runs: root Run %q has a live registry entry without a Run-tree owner",
			plan.root.run.ID(),
		)
	}
	cleanupCtx, cancel := entry.owner.cleanupContext(ctx)
	defer cancel()
	interruptCommitted, requestErr := entry.owner.requestCancel(
		cleanupCtx,
		cmd.Reason,
		func(requestCtx context.Context) error {
			return c.rootCancellation.RequestRootCancellation(
				requestCtx, plan.executor, cmd.Reason,
			)
		},
	)
	if requestErr != nil {
		if errors.Is(requestErr, ErrSessionBusy) {
			return CancelResult{}, requestErr
		}
		if errors.Is(requestErr, ErrRunFinished) {
			return CancelResult{}, errors.Join(requestErr, entry.owner.wait(cleanupCtx))
		}
		c.segments.markCancel(plan.root.run.ID(), cmd.Reason)
		return CancelResult{}, errors.Join(requestErr, entry.owner.wait(cleanupCtx))
	}
	c.segments.markCancel(plan.root.run.ID(), cmd.Reason)
	if interruptCommitted {
		// The interrupt transaction won before cancellation. Its pump owns the
		// live admission until it has published and closed the parked segment;
		// join that boundary, then apply the durable parked cancel transaction.
		if err := entry.owner.wait(cleanupCtx); err != nil {
			return CancelResult{}, err
		}
		return c.cancelKnownParkedRun(cleanupCtx, cmd, plan.root.run, plan.executor)
	}
	// The pump owns every non-parked live teardown. requestCancel has stopped its
	// stream context; joining the Run-tree owner returns that single owner's complete
	// cleanup boundary without racing a second executor Release from this goroutine.
	if err := entry.owner.wait(cleanupCtx); err != nil {
		return CancelResult{}, err
	}
	terminal, committed := entry.owner.committedTerminalRun()
	if !committed {
		return CancelResult{}, fmt.Errorf(
			"runs: canceled live root Run %q completed without a terminal snapshot",
			plan.root.run.ID(),
		)
	}
	if terminal.State() != rundomain.Canceled {
		return CancelResult{}, fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			plan.root.run.ID(),
			terminal.State(),
		)
	}
	return rootCancelResult(terminal)
}

func (c *Coordinator) cancelLiveChild(
	ctx context.Context,
	cmd CancelCommand,
	plan cancellationPlan,
	owner *runTreeOwner,
) (CancelResult, error) {
	attempt, err := owner.beginChildCancellation(plan, cmd.Reason)
	if err != nil {
		return CancelResult{}, err
	}
	cleanupCtx, cancel := owner.cleanupContext(ctx)
	defer cancel()
	if err := c.runningSubtreeCanceler.CancelRunningSubtree(
		cleanupCtx,
		plan.executor,
		plan.target.memberID,
		cmd.Reason,
	); err != nil {
		owner.abortChildCancellation(attempt, err)
		return CancelResult{}, fmt.Errorf(
			"runs: cancel child Run %q executor subtree %q: %w",
			plan.target.run.ID(),
			plan.target.memberID,
			err,
		)
	}
	target, root, err := owner.waitChildCancellation(cleanupCtx, attempt)
	if err != nil {
		return CancelResult{}, err
	}
	targetOutcome, targetHasOutcome := target.Outcome()
	if target.ID() != plan.target.run.ID() ||
		target.State() != rundomain.Canceled ||
		!targetHasOutcome || targetOutcome != rundomain.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q committed invalid target snapshot %q in state %s",
			plan.target.run.ID(),
			target.ID(),
			target.State(),
		)
	}
	if root.ID() != plan.root.run.ID() || !root.Lineage().IsRoot() {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q returned invalid root snapshot %q",
			plan.target.run.ID(),
			root.ID(),
		)
	}
	return CancelResult{Run: target, RootRun: &root}, nil
}

// cancelWaitingChild removes one complete child subtree from an executor tree
// parked at a human boundary. It first reserves the Session + working tree,
// claims the execution lifecycle, and prepares one exact executor change that
// keeps its source tree frozen. The application projections and replacement
// executor checkpoint commit in one transaction; only after that durable
// boundary may the prepared change be applied.
//
// When another external boundary survives, the tree stays Waiting. Removing
// the final boundary instead opens continuation Segments in the same transaction
// and immediately continues the already-settled runtime tree—there is no
// synthetic human answer and no transient Running row without an owner.
func (c *Coordinator) cancelWaitingChild(
	ctx context.Context,
	cmd CancelCommand,
	initial cancellationPlan,
) (result CancelResult, err error) {
	sess, err := c.sessionReader.Get(ctx, initial.pending.SessionID)
	if err != nil {
		return CancelResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(sess.ID(), sess.Workspace().Path())
	if !ok {
		return CancelResult{}, fmt.Errorf(
			"%w: session %q or working tree %q has a run or mutation in flight",
			ErrSessionBusy,
			sess.ID(),
			sess.Workspace().Path(),
		)
	}
	defer runAdmission.Release()

	cleanupCtx, cancelCleanup := (*runTreeOwner)(nil).cleanupContext(ctx)
	defer cancelCleanup()

	plan, err := c.resolveClaimedWaitingChildCancellation(cleanupCtx, cmd)
	if err != nil {
		return CancelResult{}, err
	}

	continuation, err := c.waitingChildCancellationContinuation(cleanupCtx, sess, plan)
	if err != nil {
		return CancelResult{}, err
	}
	prepared, err := c.waitingSubtreeCancellationPreparer.PrepareWaitingSubtreeCancellation(
		cleanupCtx,
		WaitingSubtreeCancellationRequest{
			Continuation: continuation, TargetMemberID: plan.target.memberID, Reason: cmd.Reason,
		},
	)
	if err != nil {
		return CancelResult{}, err
	}
	if err := prepared.Validate(); err != nil {
		return CancelResult{}, err
	}
	defer func() {
		if discardErr := prepared.Change.Discard(); discardErr != nil {
			result = CancelResult{}
			err = errors.Join(
				err,
				fmt.Errorf("runs: discard waiting subtree change: %w", discardErr),
			)
		}
	}()

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		cmd.Reason,
		c.publications.nowUTC(),
		prepared,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if transformation.remaining != nil {
		return c.commitWaitingChildCancellation(cleanupCtx, plan, transformation, prepared.Change)
	}
	return c.resumeAfterWaitingChildCancellation(
		cleanupCtx,
		sess,
		plan,
		transformation,
		prepared.Change,
		&runAdmission,
	)
}

func (c *Coordinator) resolveClaimedWaitingChildCancellation(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, error) {
	// The first plan only found the Session to reserve. Resolve it again under
	// admission so Resume or a sibling mutation cannot leave this command acting
	// on stale Pending or Item facts.
	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return cancellationPlan{}, err
	}
	if live {
		if entry.owner == nil {
			return cancellationPlan{}, fmt.Errorf(
				"runs: waiting child Run %q has a live registry entry without an owner",
				cmd.RunID,
			)
		}
		// A just-committed park releases admission immediately before its pump
		// removes the live registry entry. Join that already-terminal boundary and
		// then resolve the now-authoritative parked facts.
		if err := entry.owner.wait(ctx); err != nil {
			return cancellationPlan{}, err
		}
		plan, _, live, err = c.cancellationPlanFor(ctx, cmd)
		if err != nil {
			return cancellationPlan{}, err
		}
		if live {
			return cancellationPlan{}, fmt.Errorf(
				"runs: waiting child Run %q retained a live segment after its parked boundary completed",
				cmd.RunID,
			)
		}
	}
	if plan.treeState != rundomain.Waiting || !plan.target.run.Lineage().IsChild() {
		return cancellationPlan{}, fmt.Errorf(
			"runs: waiting child cancellation for %q resolved a %s root/child state",
			cmd.RunID,
			plan.treeState,
		)
	}
	return plan, nil
}

func (c *Coordinator) waitingChildCancellationContinuation(
	ctx context.Context,
	sess session.Session,
	plan cancellationPlan,
) (WaitingContinuation, error) {
	rootContinuation, ok := plan.pending.RootContinuation()
	if !ok {
		return WaitingContinuation{}, errors.New("runs: waiting subtree Pending has no root continuation")
	}
	checkpoint, err := c.checkpoints.ReadWaitingCheckpoint(ctx, rootContinuation.MemberID)
	if err != nil {
		if !errors.Is(err, ErrExecutorStateLost) && !errors.Is(err, ErrExecutorCheckpointNotFound) {
			return WaitingContinuation{}, err
		}
		lostErr := c.terminations.ApplyRunLost(
			ctx,
			plan.pending.SessionID,
			plan.root.run.ID(),
			c.publications.nowUTC(),
		)
		if lostErr != nil {
			return WaitingContinuation{}, errors.Join(
				err,
				fmt.Errorf("runs: recover lost waiting tree %q: %w", plan.root.run.ID(), lostErr),
			)
		}
		return WaitingContinuation{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
	}
	if err := validateCheckpointSessionScope(checkpoint, sess); err != nil {
		return WaitingContinuation{}, err
	}
	return waitingContinuationFromPending(plan.pending, checkpoint)
}

func (c *Coordinator) commitWaitingChildCancellation(
	ctx context.Context,
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
	change WaitingSubtreeChange,
) (CancelResult, error) {
	durable := transformation.durableCommit(plan.pending, newRunCommitID())
	result, err := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(
		ctx,
		durable,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if err := change.Apply(WaitingSubtreeStaysWaiting); err != nil {
		applyErr := fmt.Errorf(
			"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
			plan.target.run.ID(),
			plan.root.run.ID(),
			err,
		)
		// The database transaction is authoritative. Tear down the obsolete
		// execution and restore only from its committed resulting checkpoint.
		if recoveryErr := c.recoverCommittedWaitingCancellation(ctx, plan, transformation); recoveryErr != nil {
			return CancelResult{}, errors.Join(applyErr, recoveryErr)
		}
	}
	if err := validateWaitingChildCancellationResult(plan, result); err != nil {
		return CancelResult{}, err
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: result.TargetRun, RootRun: &result.RootRun}, nil
}

func (c *Coordinator) resumeAfterWaitingChildCancellation(
	ctx context.Context,
	sess session.Session,
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
	change WaitingSubtreeChange,
	runAdmission *sessionadmission.RunAdmission,
) (CancelResult, error) {
	rootContinuation, ok := transformation.continuation.root()
	if !ok {
		return CancelResult{}, errors.New("runs: waiting child cancellation continuation has no root Run")
	}
	segmentID := c.newSegmentID()
	if segmentID == "" {
		return CancelResult{}, errors.New("runs: waiting child cancellation generated an empty root segment id")
	}
	var committed WaitingSubtreeCancellationResult
	events, err := c.openSegment(ctx, segmentSpec{
		RunID:             plan.root.run.ID(),
		SegmentID:         segmentID,
		SessionID:         plan.pending.SessionID,
		CWD:               sess.Workspace().Path(),
		ExecutorID:        plan.executor.ExecutorID,
		ModelSelection:    rootContinuation.ModelSelection,
		GoalIncarnationID: transformation.continuation.goalIncarnationID,
		CreatedAt:         rootContinuation.RunCreatedAt,
		Continuation:      transformation.continuation,
		admission:         runAdmission,
		CommitOpening: func(commitCtx context.Context, opening OpeningCommit) error {
			if opening.Admit != nil || opening.Resume == nil {
				return errors.New("runs: waiting child continuation produced an invalid opening disposition")
			}
			durable := transformation.durableCommit(plan.pending, opening.CommitID)
			durable.Resume = opening.Resume
			durable.OpeningEvents = opening.Events
			result, commitErr := c.waitingSubtreeCancellations.CommitWaitingSubtreeCancellation(commitCtx, durable)
			if commitErr == nil {
				committed = result
			}
			return commitErr
		},
		BeginExecution: func(beginCtx context.Context) error {
			if err := change.Apply(WaitingSubtreeResumesRunning); err != nil {
				return fmt.Errorf(
					"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
					plan.target.run.ID(),
					plan.root.run.ID(),
					err,
				)
			}
			if err := change.Continue(beginCtx); err != nil {
				return fmt.Errorf(
					"runs: continue root Run %q after canceling final waiting child Run %q: %w",
					plan.root.run.ID(),
					plan.target.run.ID(),
					err,
				)
			}
			return nil
		},
	})
	if err != nil {
		return CancelResult{}, err
	}
	if events == nil {
		return CancelResult{}, errors.New("runs: waiting child continuation opened without an event stream")
	}
	if err := validateWaitingChildCancellationResult(plan, committed); err != nil {
		return CancelResult{}, err
	}
	c.publishWaitingChildCancellation(plan, transformation)
	return CancelResult{Run: committed.TargetRun, RootRun: &committed.RootRun}, nil
}

func (c *Coordinator) recoverCommittedWaitingCancellation(
	ctx context.Context,
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
) error {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if transformation.remaining == nil {
		return errors.New("runs: committed waiting cancellation recovery has no waiting boundary")
	}
	continuation, err := waitingContinuationFromPending(
		*transformation.remaining,
		transformation.checkpoint,
	)
	if err != nil {
		return c.failCommittedWaitingCancellationRecovery(recoveryCtx, plan, err)
	}
	if err := c.segments.release(recoveryCtx, plan.executor); err != nil &&
		!errors.Is(err, ErrExecutorNotLive) {
		return fmt.Errorf(
			"runs: release obsolete executor %q before restoring committed root Run %q: %w",
			plan.executor.ExecutorID,
			plan.root.run.ID(),
			err,
		)
	}
	restored, err := c.waitingRestorer.RestoreWaitingExecution(recoveryCtx, continuation)
	if err == nil {
		err = restored.ValidateFor(plan.pending.SessionID)
	}
	if err == nil && restored.ExecutorID != plan.executor.ExecutorID {
		err = fmt.Errorf(
			"restored executor %q differs from committed executor %q",
			restored.ExecutorID,
			plan.executor.ExecutorID,
		)
	}
	if err != nil {
		return c.failCommittedWaitingCancellationRecovery(recoveryCtx, plan, err)
	}
	return nil
}

func (c *Coordinator) failCommittedWaitingCancellationRecovery(
	ctx context.Context,
	plan cancellationPlan,
	cause error,
) error {
	if err := c.terminations.ApplyRunLost(
		ctx,
		plan.pending.SessionID,
		plan.root.run.ID(),
		c.publications.nowUTC(),
	); err != nil {
		return errors.Join(
			fmt.Errorf(
				"runs: restore committed waiting tree for root Run %q: %w",
				plan.root.run.ID(),
				cause,
			),
			fmt.Errorf("runs: mark unrecoverable root Run %q lost: %w", plan.root.run.ID(), err),
		)
	}
	return fmt.Errorf(
		"runs: restore committed waiting tree for root Run %q: %w",
		plan.root.run.ID(),
		cause,
	)
}

func validateWaitingChildCancellationResult(
	plan cancellationPlan,
	result WaitingSubtreeCancellationResult,
) error {
	target := result.TargetRun
	targetOutcome, targetHasOutcome := target.Outcome()
	if target.ID() != plan.target.run.ID() ||
		target.State() != rundomain.Canceled ||
		!targetHasOutcome || targetOutcome != rundomain.OutcomeCanceled {
		return fmt.Errorf(
			"runs: waiting child cancellation for %q committed invalid target snapshot %q in state %s",
			plan.target.run.ID(),
			target.ID(),
			target.State(),
		)
	}
	root := result.RootRun
	if root.ID() != plan.root.run.ID() ||
		!root.Lineage().IsRoot() ||
		(root.State() != rundomain.Waiting && root.State() != rundomain.Running) {
		return fmt.Errorf(
			"runs: waiting child cancellation for %q committed invalid root snapshot %q in state %s",
			plan.target.run.ID(),
			root.ID(),
			root.State(),
		)
	}
	return nil
}

func (c *Coordinator) publishWaitingChildCancellation(
	plan cancellationPlan,
	transformation waitingCancellationTransformation,
) {
	affected := make([]string, 0, len(transformation.canceledRunIDs)+len(plan.survivingTree)+2)
	seen := make(map[string]struct{}, cap(affected))
	appendRunID := func(runID string) {
		if runID == "" {
			return
		}
		if _, duplicate := seen[runID]; duplicate {
			return
		}
		seen[runID] = struct{}{}
		affected = append(affected, runID)
	}
	for _, runID := range transformation.canceledRunIDs {
		appendRunID(runID)
	}
	appendRunID(plan.target.run.Lineage().ParentRunID)
	if transformation.remaining == nil {
		for _, continuation := range transformation.continuation.continuations {
			appendRunID(continuation.RunID)
		}
	}
	appendRunID(plan.root.run.ID())
	c.publications.publishWaitingSubtreeCanceled(
		plan.pending.SessionID,
		plan.root.run.ID(),
		affected,
	)
}

// cancelWithoutLiveSegment resolves the small window in which a segment has
// left the live registry after the first durable read. A second durable read
// classifies a real terminal race; a still-running orphan is an invariant fault,
// never run_not_found.
func (c *Coordinator) cancelWithoutLiveSegment(ctx context.Context, cmd CancelCommand, value rundomain.Run) (CancelResult, error) {
	if value.State() == rundomain.Waiting {
		cleanupCtx, cancel := (*runTreeOwner)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, value)
	}
	refreshed, found, err := c.runs.Run(ctx, cmd.RunID)
	switch {
	case err != nil:
		return CancelResult{}, err
	case !found:
		return CancelResult{}, fmt.Errorf("runs: run %q disappeared after it was resolved", cmd.RunID)
	case refreshed.State().IsTerminal():
		return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State())
	case refreshed.State() == rundomain.Waiting:
		cleanupCtx, cancel := (*runTreeOwner)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, refreshed)
	case refreshed.State() == rundomain.Running:
		return CancelResult{}, fmt.Errorf(
			"runs: run %q is running segment %q with no live owner",
			cmd.RunID, refreshed.ActiveSegmentID(),
		)
	default:
		return CancelResult{}, fmt.Errorf("runs: run %q has unknown state %q", cmd.RunID, refreshed.State())
	}
}

// cancelParkedRun claims the Session before resolving its open interrupt. That
// admission order linearizes cancel against resume: whichever command owns the
// Session decides the one durable transition, and the loser observes busy or
// the resulting terminal state instead of misreporting run_not_found.
func (c *Coordinator) cancelParkedRun(ctx context.Context, cmd CancelCommand, value rundomain.Run) (CancelResult, error) {
	releaseSession, ok := c.admission.AcquireSession(value.SessionID())
	if !ok {
		return CancelResult{}, ErrSessionBusy
	}
	defer releaseSession()

	pending, found, err := c.interrupts.LookupOpenInterrupt(ctx, cmd.RunID)
	if err != nil {
		return CancelResult{}, err
	}
	if !found {
		refreshed, exists, lookupErr := c.runs.Run(ctx, cmd.RunID)
		switch {
		case lookupErr != nil:
			return CancelResult{}, lookupErr
		case !exists:
			return CancelResult{}, fmt.Errorf("runs: parked run %q disappeared while its session was claimed", cmd.RunID)
		case refreshed.State().IsTerminal():
			return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State())
		default:
			return CancelResult{}, fmt.Errorf("runs: run %q is %s but has no open interrupt", cmd.RunID, refreshed.State())
		}
	}
	if pending.SessionID != value.SessionID() {
		return CancelResult{}, fmt.Errorf(
			"runs: run %q belongs to session %q but its interrupt belongs to %q",
			cmd.RunID, value.SessionID(), pending.SessionID,
		)
	}
	return c.cancelClaimedParkedRun(ctx, cmd, ExecutorRef{
		SessionID:  pending.SessionID,
		ExecutorID: pending.ExecutorID,
	})
}

// cancelKnownParkedRun is used only after the live Run-tree owner proves its
// interrupt transaction committed. The owner's Segment binding is therefore the exact
// executor that transaction parked; resolving it again would introduce a second,
// weaker source of identity between two halves of one command.
func (c *Coordinator) cancelKnownParkedRun(
	ctx context.Context,
	cmd CancelCommand,
	value rundomain.Run,
	ref ExecutorRef,
) (CancelResult, error) {
	if ref.SessionID != value.SessionID() {
		return CancelResult{}, fmt.Errorf(
			"runs: Run %q belongs to Session %q but its live executor belongs to %q",
			cmd.RunID, value.SessionID(), ref.SessionID,
		)
	}
	releaseSession, ok := c.admission.AcquireSession(value.SessionID())
	if !ok {
		return CancelResult{}, ErrSessionBusy
	}
	defer releaseSession()
	return c.cancelClaimedParkedRun(ctx, cmd, ref)
}

func (c *Coordinator) cancelClaimedParkedRun(ctx context.Context, cmd CancelCommand, ref ExecutorRef) (CancelResult, error) {
	terminal, err := c.terminations.ApplyRunCancel(ctx, ref.SessionID, cmd.RunID, cmd.Reason, c.publications.nowUTC())
	if err != nil {
		return CancelResult{}, err
	}
	// The abandon write-set publishes its own invalidation: it is the transaction that
	// ends the run and drops the interrupt, and it is reached from here and from a
	// resume that finds the park unresumable. Signaling here too would be a second
	// author for one commit.
	if err := c.segments.release(ctx, ref); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		return CancelResult{}, fmt.Errorf("runs: clean up canceled parked Run %q executor: %w", cmd.RunID, err)
	}
	return rootCancelResult(terminal)
}

func rootCancelResult(value rundomain.Run) (CancelResult, error) {
	if value.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf("runs: canceled root result %q is a child Run", value.ID())
	}
	outcome, hasOutcome := value.Outcome()
	if value.State() != rundomain.Canceled || !hasOutcome || outcome != rundomain.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf("runs: cancel committed invalid terminal Run %q in state %s", value.ID(), value.State())
	}
	return CancelResult{Run: value}, nil
}
