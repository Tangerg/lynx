package runsegment

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// CommitWaitingSubtreeCancellation claims the prepared Pending snapshot and
// persists the application-defined replacement atomically. It does not decide
// which Runs survive or how their transcript and continuation facts change.
func (e *Effects) CommitWaitingSubtreeCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) (runs.WaitingSubtreeCancellationResult, error) {
	if err := e.requireWaitingCancellationStores(); err != nil {
		return runs.WaitingSubtreeCancellationResult{}, err
	}
	if err := commit.Validate(); err != nil {
		return runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: invalid waiting subtree cancellation: %w",
			err,
		)
	}
	var target, root run.Run
	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.claimWaitingCancellation(ctx, commit); err != nil {
			return err
		}
		if err := e.persistWaitingCancellationProjection(ctx, commit); err != nil {
			return err
		}
		terminalByID, err := e.terminalizeWaitingCancellationRuns(ctx, commit.TerminalRuns)
		if err != nil {
			return err
		}
		var targetFound bool
		target, targetFound = terminalByID[commit.TargetRunID]
		if !targetFound {
			return fmt.Errorf("runsegment: target Run %q was not terminalized", commit.TargetRunID)
		}

		root = commit.RootRun
		return e.persistWaitingCancellationDisposition(ctx, commit, &root)
	})
	if err != nil {
		settled, committed, settleErr := e.reconcileWaitingCancellation(ctx, commit)
		if settled {
			return committed, nil
		}
		if settleErr != nil {
			err = errors.Join(err, settleErr)
		}
		return runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: commit waiting child Run %q cancellation in root Run %q: %w",
			commit.TargetRunID,
			commit.RootRunID,
			err,
		)
	}
	return runs.WaitingSubtreeCancellationResult{TargetRun: target, RootRun: root}, nil
}

func (e *Effects) claimWaitingCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	pending, found, err := e.interrupts.Consume(ctx, commit.SessionID, commit.RootRunID)
	if err != nil {
		return fmt.Errorf(
			"runsegment: claim waiting cancellation interrupt for root Run %q: %w",
			commit.RootRunID,
			err,
		)
	}
	if !found {
		return fmt.Errorf(
			"%w: waiting cancellation interrupt for root Run %q is no longer open",
			runs.ErrSessionBusy,
			commit.RootRunID,
		)
	}
	if !pending.Equal(commit.ExpectedPending) {
		return fmt.Errorf(
			"%w: waiting cancellation interrupt for root Run %q changed after preparation",
			runs.ErrSessionBusy,
			commit.RootRunID,
		)
	}
	return nil
}

func (e *Effects) persistWaitingCancellationProjection(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	if len(commit.ConversationMessages) != 0 {
		if e.conversation == nil {
			return errors.New("runsegment: conversation persistence is unavailable")
		}
		if err := e.conversation.Write(ctx, commit.SessionID, commit.ConversationMessages...); err != nil {
			return fmt.Errorf("runsegment: append waiting cancellation conversation result: %w", err)
		}
	}
	if err := e.executorCheckpoints.SaveCheckpoint(ctx, commit.Checkpoint); err != nil {
		return fmt.Errorf(
			"runsegment: persist checkpoint for waiting child Run %q in root Run %q: %w",
			commit.TargetRunID,
			commit.RootRunID,
			err,
		)
	}
	if err := e.itemReplacer.ReplaceItem(
		ctx,
		commit.ParentItem.Expected,
		commit.ParentItem.Replacement,
	); err != nil {
		return fmt.Errorf(
			"runsegment: replace spawning Item %q for waiting child Run %q: %w",
			commit.ParentItem.Expected.ID(),
			commit.TargetRunID,
			err,
		)
	}
	for _, item := range commit.TerminalItems {
		if err := e.itemReplacer.ReplaceItem(ctx, item.Expected, item.Replacement); err != nil {
			return fmt.Errorf(
				"runsegment: settle interrupted Item %q for canceled Run %q: %w",
				item.Expected.ID(),
				item.Expected.RunID(),
				err,
			)
		}
	}
	return nil
}

func (e *Effects) terminalizeWaitingCancellationRuns(
	ctx context.Context,
	planned []run.Run,
) (map[string]run.Run, error) {
	terminalByID := make(map[string]run.Run, len(planned))
	for _, runRecord := range planned {
		finalized, err := e.finishedRun(ctx, runs.EventCommit{
			RunID:     runRecord.ID(),
			SessionID: runRecord.SessionID(),
			State:     runs.StateTerminalize,
			Outcome:   run.OutcomeCanceled,
			Run:       &runRecord,
		})
		if err != nil {
			return nil, fmt.Errorf("runsegment: finalize canceled Run %q: %w", runRecord.ID(), err)
		}
		if err := e.runState.Terminalize(ctx, finalized); err != nil {
			return nil, fmt.Errorf("runsegment: terminalize canceled Run %q: %w", runRecord.ID(), err)
		}
		terminalByID[runRecord.ID()] = finalized
	}
	return terminalByID, nil
}

func (e *Effects) persistWaitingCancellationDisposition(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
	root *run.Run,
) error {
	if root == nil {
		return errors.New("runsegment: waiting cancellation root projection is required")
	}
	if commit.RemainingPending != nil {
		if err := e.interrupts.Open(ctx, *commit.RemainingPending); err != nil {
			return fmt.Errorf(
				"runsegment: persist reduced interrupt for root Run %q: %w",
				commit.RootRunID,
				err,
			)
		}
		if err := e.runState.RecordWaitingRunCommit(
			ctx, commit.SessionID, commit.RootRunID, commit.CommitID,
		); err != nil {
			return fmt.Errorf("runsegment: record waiting cancellation commit receipt: %w", err)
		}
		return nil
	}
	for _, draft := range commit.Resume.Runs {
		if err := e.runState.Resume(ctx, commit.Resume.SessionID, draft, commit.Resume.ResumedAt); err != nil {
			return fmt.Errorf("runsegment: resume surviving Run %q: %w", draft.RunID, err)
		}
		if draft.RunID == commit.RootRunID {
			resumed, err := root.Resume(draft.SegmentID, commit.Resume.ResumedAt)
			if err != nil {
				return fmt.Errorf("runsegment: project resumed root Run %q: %w", draft.RunID, err)
			}
			*root = resumed
		}
	}
	for _, event := range commit.OpeningEvents {
		if err := e.applyCommit(ctx, event); err != nil {
			return fmt.Errorf(
				"runsegment: persist opening projection for surviving Run %q: %w",
				event.RunID,
				err,
			)
		}
	}
	segmentID, err := waitingCancellationRootSegmentID(commit)
	if err != nil {
		return err
	}
	if err := e.runState.RecordRunCommit(
		ctx, commit.SessionID, commit.RootRunID, segmentID, commit.CommitID,
	); err != nil {
		return fmt.Errorf("runsegment: record resumed waiting cancellation commit receipt: %w", err)
	}
	return nil
}

func waitingCancellationRootSegmentID(commit runs.WaitingSubtreeCancellationCommit) (string, error) {
	if commit.RemainingPending != nil {
		return "", nil
	}
	if commit.Resume == nil {
		return "", errors.New("runsegment: waiting cancellation has no surviving disposition")
	}
	for _, draft := range commit.Resume.Runs {
		if draft.RunID == commit.RootRunID {
			return draft.SegmentID, nil
		}
	}
	return "", errors.New("runsegment: waiting cancellation resume has no root Run")
}

func (e *Effects) reconcileWaitingCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) (bool, runs.WaitingSubtreeCancellationResult, error) {
	segmentID, err := waitingCancellationRootSegmentID(commit)
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, err
	}
	reconcileCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		runCommitReconciliationTimeout,
	)
	defer cancel()
	settled, err := e.runState.RunCommitCommitted(
		reconcileCtx, commit.SessionID, commit.RootRunID, segmentID, commit.CommitID,
	)
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: reconcile waiting cancellation commit: %w",
			err,
		)
	}
	if !settled {
		return false, runs.WaitingSubtreeCancellationResult{}, nil
	}
	target, found, err := e.runState.Run(reconcileCtx, commit.TargetRunID)
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: read reconciled target Run %q: %w",
			commit.TargetRunID,
			err,
		)
	}
	if !found || target.SessionID() != commit.SessionID {
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: reconciled target Run %q is unavailable",
			commit.TargetRunID,
		)
	}
	root, found, err := e.runState.Run(reconcileCtx, commit.RootRunID)
	if err != nil {
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: read reconciled root Run %q: %w",
			commit.RootRunID,
			err,
		)
	}
	if !found || root.SessionID() != commit.SessionID {
		return false, runs.WaitingSubtreeCancellationResult{}, fmt.Errorf(
			"runsegment: reconciled root Run %q is unavailable",
			commit.RootRunID,
		)
	}
	return true, runs.WaitingSubtreeCancellationResult{TargetRun: target, RootRun: root}, nil
}

func (e *Effects) requireWaitingCancellationStores() error {
	switch {
	case e.tx == nil:
		return errors.New("runsegment: transactor is unavailable")
	case e.interrupts == nil:
		return errors.New("runsegment: interrupt persistence is unavailable")
	case e.runState == nil:
		return errors.New("runsegment: run-state persistence is unavailable")
	case e.itemReplacer == nil:
		return errors.New("runsegment: transcript Item replacement is unavailable")
	case e.executorCheckpoints == nil:
		return errors.New("runsegment: executor checkpoint persistence is unavailable")
	default:
		return nil
	}
}
