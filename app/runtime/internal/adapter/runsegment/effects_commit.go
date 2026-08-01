package runsegment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// CommitOpening accepts one segment atomically. A fresh segment admits its Run;
// a continuation consumes the open interrupt and resumes the existing Run. The
// opening transcript projections land in that same transaction, so Start cannot
// acknowledge a segment whose durable opening is missing.
func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	// Exactly one admission action IS the opening's durable projection: the Run
	// comes into existence, or resumes, right here. Item commits are whatever the
	// opening happened to produce, and an approval-only resume produces none.
	if (opening.Admit == nil) == (opening.Resume == nil) {
		return errors.New("runsegment: opening requires exactly one admission action")
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		switch {
		case opening.Admit != nil:
			if e.runState == nil {
				return errors.New("runsegment: run-state persistence is unavailable")
			}
			if opening.ScheduledSession != nil {
				if opening.ScheduledSession.ID != opening.Admit.SessionID {
					return errors.New("runsegment: opening scheduled-session mismatch")
				}
				if e.sessions == nil {
					return errors.New("runsegment: session persistence is unavailable")
				}
				if _, err := e.sessions.Ensure(ctx, *opening.ScheduledSession); err != nil {
					return fmt.Errorf("runsegment: persist opening scheduled session: %w", err)
				}
			}
			if err := e.runState.Admit(ctx, *opening.Admit); err != nil {
				return err
			}
			if opening.SessionModel != nil {
				if opening.SessionModel.SessionID != opening.Admit.SessionID {
					return errors.New("runsegment: opening session-model session mismatch")
				}
				if e.sessions == nil {
					return errors.New("runsegment: session persistence is unavailable")
				}
				if err := e.sessions.SetModel(ctx, opening.SessionModel.SessionID, opening.SessionModel.Model); err != nil {
					return fmt.Errorf("runsegment: persist opening session model: %w", err)
				}
			}
			if opening.ScheduleFiring != "" {
				if e.scheduleFirings == nil {
					return errors.New("runsegment: schedule-firing persistence is unavailable")
				}
				if err := e.scheduleFirings.Accept(ctx, opening.ScheduleFiring, opening.Admit.RunID); err != nil {
					return fmt.Errorf("runsegment: accept scheduled occurrence: %w", err)
				}
			}
		case opening.Resume != nil:
			if opening.ScheduledSession != nil || opening.SessionModel != nil || opening.ScheduleFiring != "" {
				return errors.New("runsegment: resumed opening cannot carry fresh-run facts")
			}
			if err := e.consumeResume(ctx, *opening.Resume); err != nil {
				return err
			}
		}
		for _, commit := range opening.Events {
			if err := commit.Validate(); err != nil {
				return fmt.Errorf("runsegment: invalid opening event commit: %w", err)
			}
			if commit.State != runs.StateUnchanged {
				return errors.New("runsegment: opening commit contains a lifecycle transition")
			}
			if len(commit.Items) == 0 {
				return errors.New("runsegment: opening commit has no items to append")
			}
			if err := e.applyCommit(ctx, commit); err != nil {
				return err
			}
		}
		return nil
	})
}

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// transcript item/run projections and the run-state transition in one
// transaction. Tree suspension is deliberately excluded: it must use
// CommitTreeBarrier so no individual Run can publish a partial barrier.
func (e *Effects) CommitEvent(ctx context.Context, commit runs.EventCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	if err := commit.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid event commit: %w", err)
	}
	if commit.State == runs.StateSuspend {
		return errors.New("runsegment: per-Run suspend commit is not allowed")
	}
	if commit.ObsoleteCheckpointRootID != "" && commit.State != runs.StateTerminalize {
		return errors.New("runsegment: executor checkpoint deletion requires a terminal Run commit")
	}
	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
		if commit.ObsoleteCheckpointRootID == "" {
			return nil
		}
		if e.executorCheckpoints == nil {
			return errors.New("runsegment: executor checkpoint persistence is unavailable")
		}
		if err := e.executorCheckpoints.DeleteCheckpoints(ctx, commit.SessionID, []string{commit.ObsoleteCheckpointRootID}); err != nil {
			return fmt.Errorf("runsegment: delete terminal executor checkpoint %q: %w", commit.ObsoleteCheckpointRootID, err)
		}
		return nil
	})
	if err != nil {
		return e.compensateFailedCommit(ctx, commit, err)
	}
	return nil
}

// CommitTreeBarrier atomically records one root-owned pending set and suspends
// every active Run named by it. The caller supplies Runs in protocol publication
// order; persistence preserves that order while the transaction makes it
// invisible until complete.
func (e *Effects) CommitTreeBarrier(ctx context.Context, barrier runs.TreeBarrierCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	if e.executorCheckpoints == nil {
		return errors.New("runsegment: executor checkpoint persistence is unavailable")
	}
	if err := barrier.Pending.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid tree barrier: %w", err)
	}
	rootContinuation, ok := barrier.Pending.RootContinuation()
	if !ok {
		return errors.New("runsegment: tree barrier has no root continuation")
	}
	if err := barrier.Checkpoint.ValidateOwnership(
		rootContinuation.ProcessID,
		barrier.Pending.SessionID,
	); err != nil {
		return fmt.Errorf("runsegment: invalid tree barrier executor checkpoint ownership: %w", err)
	}
	if barrier.Checkpoint.Scope.GoalLeaseID != barrier.Pending.GoalLeaseID {
		return fmt.Errorf(
			"runsegment: tree barrier checkpoint goal lease %q does not match Pending %q: %w",
			barrier.Checkpoint.Scope.GoalLeaseID,
			barrier.Pending.GoalLeaseID,
			execution.ErrInvalidExecutorCheckpoint,
		)
	}
	if barrier.Checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return fmt.Errorf(
			"runsegment: tree barrier checkpoint model %q/%q does not match root continuation %q/%q: %w",
			barrier.Checkpoint.ModelSelection.Provider(),
			barrier.Checkpoint.ModelSelection.Model(),
			rootContinuation.ModelSelection.Provider(),
			rootContinuation.ModelSelection.Model(),
			execution.ErrInvalidExecutorCheckpoint,
		)
	}
	if barrier.Checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf(
			"runsegment: tree barrier checkpoint limits %+v do not match root continuation %+v: %w",
			barrier.Checkpoint.Limits,
			rootContinuation.Limits,
			execution.ErrInvalidExecutorCheckpoint,
		)
	}
	if len(barrier.Runs) != len(barrier.Pending.Continuations) {
		return fmt.Errorf(
			"runsegment: tree barrier has %d Run commits for %d continuations",
			len(barrier.Runs),
			len(barrier.Pending.Continuations),
		)
	}
	continuations := make(map[string]interrupts.Continuation, len(barrier.Pending.Continuations))
	for _, continuation := range barrier.Pending.Continuations {
		continuations[continuation.RunID] = continuation
	}
	seen := make(map[string]struct{}, len(barrier.Runs))
	for index, commit := range barrier.Runs {
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runsegment: tree barrier Run[%d]: %w", index, err)
		}
		switch {
		case commit.State != runs.StateSuspend:
			return fmt.Errorf("runsegment: tree barrier Run[%d] is not a suspend commit", index)
		case commit.Run == nil || commit.Run.State != execution.Interrupted:
			return fmt.Errorf("runsegment: tree barrier Run[%d] has no interrupted Run", index)
		case commit.RunID != commit.Run.ID:
			return fmt.Errorf("runsegment: tree barrier Run[%d] identity mismatch", index)
		case commit.SessionID != barrier.Pending.SessionID ||
			commit.Run.SessionID != barrier.Pending.SessionID:
			return fmt.Errorf("runsegment: tree barrier Run[%d] session mismatch", index)
		case commit.GoalTurn != nil:
			return fmt.Errorf("runsegment: tree barrier Run[%d] carries a terminal goal charge", index)
		}
		continuation, exists := continuations[commit.RunID]
		if !exists {
			return fmt.Errorf("runsegment: tree barrier Run[%d] has no continuation", index)
		}
		if commit.Run.Lineage() != continuation.Lineage ||
			commit.Run.ModelSelection != continuation.ModelSelection ||
			!commit.Run.CreatedAt.Equal(continuation.RunCreatedAt) ||
			!commit.Run.Metrics.Equal(continuation.Metrics) ||
			commit.Run.Limits != continuation.Limits {
			return fmt.Errorf("runsegment: tree barrier Run[%d] differs from its continuation", index)
		}
		if !commit.Run.ProtocolProfile.Equal(barrier.Pending.ProtocolProfile) {
			return fmt.Errorf("runsegment: tree barrier Run[%d] protocol profile differs from Pending", index)
		}
		if commit.RunID == barrier.Pending.RootRunID {
			if commit.Run.GoalLeaseID != barrier.Pending.GoalLeaseID {
				return fmt.Errorf("runsegment: tree barrier root Run goal lease differs from Pending")
			}
		} else if commit.Run.GoalLeaseID != "" {
			return fmt.Errorf("runsegment: tree barrier child Run[%d] carries a root Goal lease", index)
		}
		if _, duplicate := seen[commit.RunID]; duplicate {
			return fmt.Errorf("runsegment: tree barrier repeats Run %q", commit.RunID)
		}
		seen[commit.RunID] = struct{}{}
	}

	err := e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.executorCheckpoints.SaveCheckpoint(ctx, barrier.Checkpoint); err != nil {
			return fmt.Errorf("runsegment: persist tree barrier executor checkpoint: %w", err)
		}
		if err := e.openInterrupt(ctx, barrier.Pending); err != nil {
			return err
		}
		for _, commit := range barrier.Runs {
			if err := e.applyCommit(ctx, commit); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return nil
	}
	for _, commit := range barrier.Runs {
		err = e.compensateFailedCommit(ctx, commit, err)
	}
	return err
}

const stagedToolResultCleanupTimeout = 5 * time.Second

// compensateFailedCommit removes only unbound blobs staged by the failed
// event. Cleanup is request-detached because cancellation is one of the failure
// paths; Discard's unbound predicate makes an ambiguous successful commit safe.
func (e *Effects) compensateFailedCommit(ctx context.Context, commit runs.EventCommit, commitErr error) error {
	if e.toolResults == nil {
		return commitErr
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stagedToolResultCleanupTimeout)
	defer cancel()
	var cleanupErrs []error
	for _, item := range commit.Items {
		if item.Tool == nil || item.Tool.Offload == nil {
			continue
		}
		if err := e.toolResults.Discard(cleanupCtx, item.SessionID, *item.Tool.Offload); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("runsegment: discard staged tool result %q: %w", item.Tool.Offload.ID, err))
		}
	}
	return errors.Join(commitErr, errors.Join(cleanupErrs...))
}

func (e *Effects) applyCommit(ctx context.Context, commit runs.EventCommit) error {
	for _, item := range commit.Items {
		if err := e.appendItem(ctx, item); err != nil {
			return err
		}
	}
	if err := e.applyState(ctx, commit); err != nil {
		return err
	}
	if commit.GoalTurn != nil {
		if e.goalTurns == nil {
			return errors.New("runsegment: goal-turn persistence is unavailable")
		}
		if err := e.goalTurns.RecordTurn(ctx, *commit.GoalTurn); err != nil {
			return fmt.Errorf("runsegment: record goal turn: %w", err)
		}
	}
	return nil
}

func (e *Effects) consumeResume(ctx context.Context, resume execution.TreeResumeDraft) error {
	if err := resume.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid tree resume: %w", err)
	}
	if e.interrupts == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	pending, ok, err := e.interrupts.Consume(ctx, resume.SessionID, resume.RootRunID)
	if err != nil {
		return fmt.Errorf("runsegment: consume resume interrupt: %w", err)
	}
	if !ok {
		return errors.New("runsegment: resume interrupt is no longer open")
	}
	if pending.SessionID != resume.SessionID {
		return fmt.Errorf("runsegment: resume interrupt session mismatch: got %q want %q", pending.SessionID, resume.SessionID)
	}
	if pending.RootRunID != resume.RootRunID {
		return fmt.Errorf(
			"runsegment: consumed interrupt root mismatch: got %q want %q",
			pending.RootRunID,
			resume.RootRunID,
		)
	}
	if err := pending.Validate(); err != nil {
		return fmt.Errorf("runsegment: consumed invalid interrupt set: %w", err)
	}
	if len(pending.Continuations) != len(resume.Runs) {
		return fmt.Errorf(
			"runsegment: tree resume has %d Runs for %d continuations",
			len(resume.Runs),
			len(pending.Continuations),
		)
	}
	for index, continuation := range pending.Continuations {
		if resume.Runs[index].RunID != continuation.RunID {
			return fmt.Errorf(
				"runsegment: tree resume Run[%d] is %q, pending postorder requires %q",
				index,
				resume.Runs[index].RunID,
				continuation.RunID,
			)
		}
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	for _, run := range resume.Runs {
		if err := e.runState.Resume(ctx, resume.SessionID, run, resume.ResumedAt); err != nil {
			return fmt.Errorf("runsegment: resume Run %q state: %w", run.RunID, err)
		}
	}
	return nil
}

func (e *Effects) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return e.tx(ctx, fn)
}

func (e *Effects) openInterrupt(ctx context.Context, p interrupts.Pending) error {
	if e.interrupts == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	if err := e.interrupts.Open(ctx, p); err != nil {
		return fmt.Errorf("runsegment: persist interrupt: %w", err)
	}
	return nil
}

func (e *Effects) appendItem(ctx context.Context, item transcript.Item) error {
	if e.transcript == nil {
		return errors.New("runsegment: transcript persistence is unavailable")
	}
	if err := e.transcript.AppendItem(ctx, item); err != nil {
		return err
	}
	if item.Tool == nil || item.Tool.Offload == nil {
		return nil
	}
	if item.Tool.Result == nil {
		return errors.New("runsegment: offloaded tool result is absent")
	}
	preview, ok := item.Tool.Result.String()
	if !ok {
		return errors.New("runsegment: offloaded tool result has no preview string")
	}
	if e.toolResults == nil {
		return errors.New("runsegment: tool-result persistence is unavailable")
	}
	if err := e.toolResults.Bind(ctx, item.SessionID, item.ID, preview, *item.Tool.Offload); err != nil {
		return fmt.Errorf("runsegment: bind offloaded tool result: %w", err)
	}
	return nil
}

func (e *Effects) applyState(ctx context.Context, commit runs.EventCommit) error {
	if commit.State == runs.StateUnchanged {
		return nil
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	switch commit.State {
	case runs.StateSuspend:
		if commit.Run == nil {
			return errors.New("runsegment: park commit carries no run record")
		}
		return e.runState.Suspend(ctx, *commit.Run)
	case runs.StateTerminalize:
		run, err := e.finishedRun(ctx, commit)
		if err != nil {
			return err
		}
		return e.runState.Terminalize(ctx, run)
	default:
		return fmt.Errorf("runsegment: unknown run state change %d", commit.State)
	}
}

// finishedRun completes the terminal Run record with the two facts the reducer
// cannot know: the conversation watermark, resolved inside the caller's
// transaction so it is consistent with the state it terminalizes (the message log
// is in its terminal post-compaction shape by the time a terminal event arrives),
// and the row's touch time.
func (e *Effects) finishedRun(ctx context.Context, commit runs.EventCommit) (transcript.Run, error) {
	if commit.Run == nil {
		return transcript.Run{}, errors.New("runsegment: terminal commit carries no run record")
	}
	run := *commit.Run
	if run.MessageMark < 0 {
		if e.messages == nil {
			return transcript.Run{}, errors.New("runsegment: message persistence is unavailable")
		}
		mark, err := e.messages.Count(ctx, run.SessionID)
		if err != nil {
			return transcript.Run{}, fmt.Errorf("runsegment: resolve terminal message watermark: %w", err)
		}
		run.MessageMark = mark
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	return run, nil
}
