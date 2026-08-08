package runs

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Cancel handles both live and parked runs under the same run/session admission
// rules and returns the exact terminal Run committed by the winning write-set.
// The durable abandon write-set is authoritative and commits before a parked
// executor is torn down. Cleanup errors are returned unless the executor already
// disappeared, which is the idempotent completion race.
func (c *Coordinator) Cancel(ctx context.Context, cmd CancelCommand) (CancelResult, error) {
	if err := c.requireControlDependencies(); err != nil {
		return CancelResult{}, err
	}

	plan, entry, live, err := c.cancellationPlanFor(ctx, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	if plan.target.run.Lineage().IsChild() {
		switch plan.treeState {
		case rundomain.Running:
			if !live || entry.handle == nil {
				return CancelResult{}, fmt.Errorf(
					"runs: running child Run %q has no live root owner",
					cmd.RunID,
				)
			}
			return c.cancelLiveChild(ctx, cmd, plan, entry.handle)
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
	if entry.handle == nil {
		return CancelResult{}, fmt.Errorf(
			"runs: root Run %q has a live registry entry without a handle",
			plan.root.run.ID,
		)
	}
	cleanupCtx, cancel := entry.handle.cleanupContext(ctx)
	defer cancel()
	interruptCommitted, requestErr := entry.handle.requestCancel(cleanupCtx, cmd.Reason)
	if requestErr != nil {
		if errors.Is(requestErr, ErrSessionBusy) {
			return CancelResult{}, requestErr
		}
		c.registry.MarkCancel(plan.root.run.ID, cmd.Reason)
		return CancelResult{}, errors.Join(requestErr, entry.handle.wait(cleanupCtx))
	}
	c.registry.MarkCancel(plan.root.run.ID, cmd.Reason)
	if interruptCommitted {
		// The interrupt transaction won before cancellation. Its pump owns the
		// live admission until it has published and closed the parked segment;
		// join that boundary, then apply the durable parked cancel transaction.
		if err := entry.handle.wait(cleanupCtx); err != nil {
			return CancelResult{}, err
		}
		return c.cancelKnownParkedRun(cleanupCtx, cmd, plan.root.run, plan.executor)
	}
	// The pump owns every non-parked live teardown. requestCancel has stopped its
	// stream context; joining the handle returns that single owner's complete
	// cleanup boundary without racing a second executor Release from this goroutine.
	if err := entry.handle.wait(cleanupCtx); err != nil {
		return CancelResult{}, err
	}
	terminal, committed := entry.handle.committedTerminalRun()
	if !committed {
		return CancelResult{}, fmt.Errorf(
			"runs: canceled live root Run %q completed without a terminal snapshot",
			plan.root.run.ID,
		)
	}
	if terminal.State != rundomain.Canceled {
		return CancelResult{}, fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			plan.root.run.ID,
			terminal.State,
		)
	}
	return rootCancelResult(terminal)
}

func (c *Coordinator) cancelLiveChild(
	ctx context.Context,
	cmd CancelCommand,
	plan cancellationPlan,
	live *handle,
) (CancelResult, error) {
	attempt, err := live.beginChildCancellation(plan, cmd.Reason)
	if err != nil {
		return CancelResult{}, err
	}
	cleanupCtx, cancel := live.cleanupContext(ctx)
	defer cancel()
	if err := c.runningTrees.CancelRunningSubtree(
		cleanupCtx,
		plan.executor,
		plan.target.memberID,
	); err != nil {
		live.abortChildCancellation(attempt, err)
		return CancelResult{}, fmt.Errorf(
			"runs: cancel child Run %q executor subtree %q: %w",
			plan.target.run.ID,
			plan.target.memberID,
			err,
		)
	}
	target, root, err := live.waitChildCancellation(cleanupCtx, attempt)
	if err != nil {
		return CancelResult{}, err
	}
	if target.ID != plan.target.run.ID ||
		target.State != rundomain.Canceled ||
		target.Outcome == nil ||
		*target.Outcome != rundomain.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q committed invalid target snapshot %q in state %s",
			plan.target.run.ID,
			target.ID,
			target.State,
		)
	}
	if root.ID != plan.root.run.ID || !root.Lineage().IsRoot() {
		return CancelResult{}, fmt.Errorf(
			"runs: child cancellation for %q returned invalid root snapshot %q",
			plan.target.run.ID,
			root.ID,
		)
	}
	return CancelResult{Run: target, RootRun: &root}, nil
}

// cancelWaitingChild removes one complete child subtree from an executor tree
// parked at a human boundary. It first reserves the Session + working tree,
// claims the execution lifecycle, and obtains an immutable transition plan
// that retains no runtime ownership. The application projections and replacement
// executor checkpoint commit in one transaction; only after that durable boundary may
// the live transition be applied.
//
// When another external boundary survives, the tree stays Waiting. Removing
// the final boundary instead opens continuation Segments in the same transaction
// and immediately continues the already-settled runtime tree—there is no
// synthetic human answer and no transient Running row without an owner.
func (c *Coordinator) cancelWaitingChild(
	ctx context.Context,
	cmd CancelCommand,
	initial cancellationPlan,
) (CancelResult, error) {
	if c.waitingEdits == nil {
		return CancelResult{}, errors.New("runs: effects are required for waiting child cancellation")
	}
	if c.newSegmentID == nil {
		return CancelResult{}, errors.New("runs: segment id generator is required for waiting child cancellation")
	}
	sess, err := c.sessionReader.Get(ctx, initial.pending.SessionID)
	if err != nil {
		return CancelResult{}, err
	}
	runAdmission, ok := c.admission.AcquireRun(sess.ID, sess.CWD)
	if !ok {
		return CancelResult{}, fmt.Errorf(
			"%w: session %q or working tree %q has a run or mutation in flight",
			ErrSessionBusy,
			sess.ID,
			sess.CWD,
		)
	}
	defer runAdmission.Release()

	cleanupCtx, cancelCleanup := (*handle)(nil).cleanupContext(ctx)
	defer cancelCleanup()

	// The first plan let us find the Session to reserve. Resolve it again under
	// admission so a Resume or sibling mutation that won immediately before the
	// reservation cannot leave this command acting on stale Pending/Item facts.
	plan, entry, live, err := c.cancellationPlanFor(cleanupCtx, cmd)
	if err != nil {
		return CancelResult{}, err
	}
	if live {
		if entry.handle == nil {
			return CancelResult{}, fmt.Errorf(
				"runs: waiting child Run %q has a live registry entry without an owner",
				cmd.RunID,
			)
		}
		// A just-committed park releases admission immediately before its pump
		// removes the live registry entry. Joining that already-terminal boundary
		// closes the tiny hand-off window without treating it as an invariant
		// failure or preparing a second Segment beside the first.
		if err := entry.handle.wait(cleanupCtx); err != nil {
			return CancelResult{}, err
		}
		plan, _, live, err = c.cancellationPlanFor(cleanupCtx, cmd)
		if err != nil {
			return CancelResult{}, err
		}
		if live {
			return CancelResult{}, fmt.Errorf(
				"runs: waiting child Run %q retained a live segment after its parked boundary completed",
				cmd.RunID,
			)
		}
	}
	if plan.treeState != rundomain.Waiting || !plan.target.run.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf(
			"runs: waiting child cancellation for %q resolved a %s root/child state",
			cmd.RunID,
			plan.treeState,
		)
	}

	ref, err := c.prepareLegacyWaitingExecution(cleanupCtx, plan.pending, sess.CWD, sess.Isolated)
	if err != nil {
		if errors.Is(err, ErrExecutorStateLost) {
			lostErr := c.terminations.ApplyRunLost(
				cleanupCtx,
				plan.pending.SessionID,
				plan.root.run.ID,
				c.now().UTC(),
			)
			if lostErr != nil {
				return CancelResult{}, errors.Join(
					err,
					fmt.Errorf("runs: recover lost waiting tree %q: %w", plan.root.run.ID, lostErr),
				)
			}
			return CancelResult{}, fmt.Errorf("%w: %w", ErrRunNotFound, err)
		}
		return CancelResult{}, err
	}
	if ref != plan.executor {
		return CancelResult{}, fmt.Errorf(
			"runs: prepared executor %q/%q differs from waiting cancellation executor %q/%q",
			ref.SessionID,
			ref.ExecutorID,
			plan.executor.SessionID,
			plan.executor.ExecutorID,
		)
	}

	prepared, err := c.waitingTrees.PrepareWaitingSubtreeCancellation(
		cleanupCtx,
		ref,
		plan.target.memberID,
	)
	if err != nil {
		return CancelResult{}, err
	}
	defer prepared.Mutation.Abort()

	transformation, err := prepareWaitingCancellationTransformation(
		plan,
		cmd.Reason,
		c.now().UTC(),
		prepared,
	)
	if err != nil {
		return CancelResult{}, err
	}
	if transformation.remaining != nil {
		result, err := c.waitingEdits.CommitWaitingSubtreeCancellation(
			cleanupCtx,
			transformation.durableCommit(plan.pending),
		)
		if err != nil {
			return CancelResult{}, err
		}
		if err := prepared.Mutation.Commit(cleanupCtx, WaitingSubtreeRemainsInterrupted); err != nil {
			applyErr := fmt.Errorf(
				"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
				plan.target.run.ID,
				plan.root.run.ID,
				err,
			)
			// The database transaction is already authoritative. Release the
			// lifecycle claim before tearing down the obsolete execution tree, then
			// fail the durable tree closed as run_lost.
			prepared.Mutation.Abort()
			recoveryErr := c.recoverCommittedWaitingCancellation(cleanupCtx, plan)
			return CancelResult{}, errors.Join(applyErr, recoveryErr)
		}
		if err := validateWaitingChildCancellationResult(plan, result); err != nil {
			return CancelResult{}, err
		}
		c.publishWaitingChildCancellation(plan, transformation)
		return CancelResult{Run: result.TargetRun, RootRun: &result.RootRun}, nil
	}

	rootContinuation, ok := transformation.continuation.root()
	if !ok {
		return CancelResult{}, errors.New("runs: waiting child cancellation continuation has no root Run")
	}
	segmentID := c.newSegmentID()
	if segmentID == "" {
		return CancelResult{}, errors.New("runs: waiting child cancellation generated an empty root segment id")
	}
	var committed WaitingSubtreeCancellationResult
	events, err := c.openSegment(cleanupCtx, segmentSpec{
		RunID:          plan.root.run.ID,
		SegmentID:      segmentID,
		SessionID:      plan.pending.SessionID,
		CWD:            sess.CWD,
		ExecutorID:     ref.ExecutorID,
		ModelSelection: rootContinuation.ModelSelection,
		GoalLeaseID:    transformation.continuation.goalLeaseID,
		CreatedAt:      rootContinuation.RunCreatedAt,
		Continuation:   transformation.continuation,
		admission:      &runAdmission,
		CommitOpening: func(commitCtx context.Context, opening OpeningCommit) error {
			if opening.Admit != nil || opening.Resume == nil {
				return errors.New("runs: waiting child continuation produced an invalid opening disposition")
			}
			durable := transformation.durableCommit(plan.pending)
			durable.Resume = opening.Resume
			durable.OpeningEvents = opening.Events
			result, commitErr := c.waitingEdits.CommitWaitingSubtreeCancellation(commitCtx, durable)
			if commitErr == nil {
				committed = result
			}
			return commitErr
		},
		BeginExecution: func(beginCtx context.Context) error {
			if err := prepared.Mutation.Commit(beginCtx, WaitingSubtreeContinues); err != nil {
				return fmt.Errorf(
					"runs: apply committed cancellation of waiting child Run %q in root Run %q: %w",
					plan.target.run.ID,
					plan.root.run.ID,
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
) error {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.terminations.ApplyRunLost(
		recoveryCtx,
		plan.pending.SessionID,
		plan.root.run.ID,
		c.now().UTC(),
	); err != nil {
		return fmt.Errorf(
			"runs: recover root Run %q after its committed executor checkpoint could not be applied to the live runtime: %w",
			plan.root.run.ID,
			err,
		)
	}
	if err := c.releases.Release(recoveryCtx, plan.executor); err != nil {
		return fmt.Errorf(
			"runs: release obsolete executor %q after root Run %q was recovered lost: %w",
			plan.executor.ExecutorID,
			plan.root.run.ID,
			err,
		)
	}
	return nil
}

func validateWaitingChildCancellationResult(
	plan cancellationPlan,
	result WaitingSubtreeCancellationResult,
) error {
	target := result.TargetRun
	if target.ID != plan.target.run.ID ||
		target.State != rundomain.Canceled ||
		target.Outcome == nil ||
		*target.Outcome != rundomain.OutcomeCanceled {
		return fmt.Errorf(
			"runs: waiting child cancellation for %q committed invalid target snapshot %q in state %s",
			plan.target.run.ID,
			target.ID,
			target.State,
		)
	}
	root := result.RootRun
	if root.ID != plan.root.run.ID ||
		!root.Lineage().IsRoot() ||
		(root.State != rundomain.Waiting && root.State != rundomain.Running) {
		return fmt.Errorf(
			"runs: waiting child cancellation for %q committed invalid root snapshot %q in state %s",
			plan.target.run.ID,
			root.ID,
			root.State,
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
	appendRunID(plan.target.run.ParentRunID)
	if transformation.remaining == nil {
		for _, continuation := range transformation.continuation.continuations {
			appendRunID(continuation.RunID)
		}
	}
	appendRunID(plan.root.run.ID)
	c.publishWaitingSubtreeCanceled(
		plan.pending.SessionID,
		plan.root.run.ID,
		affected,
	)
}

// cancelWithoutLiveSegment resolves the small window in which a segment has
// left the live registry after the first durable read. A second durable read
// classifies a real terminal race; a still-running orphan is an invariant fault,
// never run_not_found.
func (c *Coordinator) cancelWithoutLiveSegment(ctx context.Context, cmd CancelCommand, run transcript.Run) (CancelResult, error) {
	if run.State == rundomain.Waiting {
		cleanupCtx, cancel := (*handle)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, run)
	}
	refreshed, found, err := c.runs.Run(ctx, cmd.RunID)
	switch {
	case err != nil:
		return CancelResult{}, err
	case !found:
		return CancelResult{}, fmt.Errorf("runs: run %q disappeared after it was resolved", cmd.RunID)
	case refreshed.State.IsTerminal():
		return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State)
	case refreshed.State == rundomain.Waiting:
		cleanupCtx, cancel := (*handle)(nil).cleanupContext(ctx)
		defer cancel()
		return c.cancelParkedRun(cleanupCtx, cmd, refreshed)
	case refreshed.State == rundomain.Running:
		return CancelResult{}, fmt.Errorf(
			"runs: run %q is running segment %q with no live owner",
			cmd.RunID, refreshed.ActiveSegmentID,
		)
	default:
		return CancelResult{}, fmt.Errorf("runs: run %q has unknown state %d", cmd.RunID, refreshed.State)
	}
}

// cancelParkedRun claims the Session before resolving its open interrupt. That
// admission order linearizes cancel against resume: whichever command owns the
// Session decides the one durable transition, and the loser observes busy or
// the resulting terminal state instead of misreporting run_not_found.
func (c *Coordinator) cancelParkedRun(ctx context.Context, cmd CancelCommand, run transcript.Run) (CancelResult, error) {
	releaseSession, ok := c.admission.AcquireSession(run.SessionID)
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
		case refreshed.State.IsTerminal():
			return CancelResult{}, fmt.Errorf("%w: %q completed as %s", ErrRunFinished, cmd.RunID, refreshed.State)
		default:
			return CancelResult{}, fmt.Errorf("runs: run %q is %s but has no open interrupt", cmd.RunID, refreshed.State)
		}
	}
	if pending.SessionID != run.SessionID {
		return CancelResult{}, fmt.Errorf(
			"runs: run %q belongs to session %q but its interrupt belongs to %q",
			cmd.RunID, run.SessionID, pending.SessionID,
		)
	}
	return c.cancelClaimedParkedRun(ctx, cmd, ExecutorRef{
		SessionID:  pending.SessionID,
		ExecutorID: pending.ExecutorID,
	})
}

// cancelKnownParkedRun is used only after the live handle proves its interrupt
// transaction committed. The handle's Segment binding is therefore the exact
// executor that transaction parked; resolving it again would introduce a second,
// weaker source of identity between two halves of one command.
func (c *Coordinator) cancelKnownParkedRun(
	ctx context.Context,
	cmd CancelCommand,
	run transcript.Run,
	ref ExecutorRef,
) (CancelResult, error) {
	if ref.SessionID != run.SessionID {
		return CancelResult{}, fmt.Errorf(
			"runs: Run %q belongs to Session %q but its live executor belongs to %q",
			cmd.RunID, run.SessionID, ref.SessionID,
		)
	}
	releaseSession, ok := c.admission.AcquireSession(run.SessionID)
	if !ok {
		return CancelResult{}, ErrSessionBusy
	}
	defer releaseSession()
	return c.cancelClaimedParkedRun(ctx, cmd, ref)
}

func (c *Coordinator) cancelClaimedParkedRun(ctx context.Context, cmd CancelCommand, ref ExecutorRef) (CancelResult, error) {
	terminal, err := c.terminations.ApplyRunCancel(ctx, ref.SessionID, cmd.RunID, cmd.Reason, c.now().UTC())
	if err != nil {
		return CancelResult{}, err
	}
	// The abandon write-set publishes its own invalidation: it is the transaction that
	// ends the run and drops the interrupt, and it is reached from here and from a
	// resume that finds the park unresumable. Signaling here too would be a second
	// author for one commit.
	if err := c.releases.Release(ctx, ref); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		return CancelResult{}, fmt.Errorf("runs: clean up canceled parked Run %q executor: %w", cmd.RunID, err)
	}
	return rootCancelResult(terminal)
}

func rootCancelResult(run transcript.Run) (CancelResult, error) {
	if run.Lineage().IsChild() {
		return CancelResult{}, fmt.Errorf("runs: canceled root result %q is a child run", run.ID)
	}
	if run.State != rundomain.Canceled || run.Outcome == nil || *run.Outcome != rundomain.OutcomeCanceled {
		return CancelResult{}, fmt.Errorf("runs: cancel committed invalid terminal run %q in state %s", run.ID, run.State)
	}
	return CancelResult{Run: run}, nil
}
