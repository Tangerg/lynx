package runsegment

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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
	var target, root transcript.Run
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
			commit.ParentItem.Expected.ID,
			commit.TargetRunID,
			err,
		)
	}
	for _, item := range commit.TerminalItems {
		if err := e.itemReplacer.ReplaceItem(ctx, item.Expected, item.Replacement); err != nil {
			return fmt.Errorf(
				"runsegment: settle interrupted Item %q for canceled Run %q: %w",
				item.Expected.ID,
				item.Expected.RunID,
				err,
			)
		}
	}
	return nil
}

func (e *Effects) terminalizeWaitingCancellationRuns(
	ctx context.Context,
	planned []transcript.Run,
) (map[string]transcript.Run, error) {
	terminalByID := make(map[string]transcript.Run, len(planned))
	for _, runRecord := range planned {
		finalized, err := e.finishedRun(ctx, runs.EventCommit{
			RunID:     runRecord.ID,
			SessionID: runRecord.SessionID,
			State:     runs.StateTerminalize,
			Outcome:   run.OutcomeCanceled,
			Run:       &runRecord,
		})
		if err != nil {
			return nil, fmt.Errorf("runsegment: finalize canceled Run %q: %w", runRecord.ID, err)
		}
		if err := e.runState.Terminalize(ctx, finalized); err != nil {
			return nil, fmt.Errorf("runsegment: terminalize canceled Run %q: %w", runRecord.ID, err)
		}
		terminalByID[runRecord.ID] = finalized
	}
	return terminalByID, nil
}

func (e *Effects) persistWaitingCancellationDisposition(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
	root *transcript.Run,
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
		root.Interrupts = directInterrupts(*commit.RemainingPending, commit.RootRunID)
		return nil
	}
	for _, draft := range commit.Resume.Runs {
		if err := e.runState.Resume(ctx, commit.Resume.SessionID, draft, commit.Resume.ResumedAt); err != nil {
			return fmt.Errorf("runsegment: resume surviving Run %q: %w", draft.RunID, err)
		}
		if draft.RunID == commit.RootRunID {
			root.State = run.Running
			root.ActiveSegmentID = draft.SegmentID
			root.Interrupts = nil
			root.UpdatedAt = commit.Resume.ResumedAt.UTC()
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
	return nil
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

func directInterrupts(pending runs.Pending, runID string) []transcript.Interrupt {
	out := make([]transcript.Interrupt, 0, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		if interrupt.RunID == runID {
			out = append(out, interrupt)
		}
	}
	return out
}
