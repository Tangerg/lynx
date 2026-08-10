package runsegment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// ReserveChildRunStart durably records one invisible child identity. It does
// not admit a Run or append Transcript content.
func (e *Effects) ReserveChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	if e.tx == nil || e.childRunStarts == nil {
		return errors.New("runsegment: child Run start persistence is unavailable")
	}
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		if err := e.childRunStarts.Reserve(ctx, record); err != nil {
			return fmt.Errorf("runsegment: reserve child Run start: %w", err)
		}
		return nil
	})
}

// CommitStartedChildRun atomically consumes the invisible reservation with the
// public Running child Run and its causal parent Item.
func (e *Effects) CommitStartedChildRun(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
	opening runs.OpeningCommit,
) error {
	if e.tx == nil || e.childRunStarts == nil {
		return errors.New("runsegment: child Run start persistence is unavailable")
	}
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	if err := validateStartedChildOpening(reservation, opening); err != nil {
		return err
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		changed, err := e.childRunStarts.Conclude(
			ctx, record, sqlite.ChildRunStartConclusionStarted,
		)
		if err != nil {
			return fmt.Errorf("runsegment: conclude started child Run reservation: %w", err)
		}
		if !changed {
			return nil
		}
		if err := e.commitOpening(ctx, opening); err != nil {
			return fmt.Errorf("runsegment: commit started child Run opening: %w", err)
		}
		return nil
	})
}

// AbortChildRunStart consumes an invisible reservation without creating a Run.
func (e *Effects) AbortChildRunStart(
	ctx context.Context,
	reservation runs.ChildRunStartReservation,
) error {
	if e.tx == nil || e.childRunStarts == nil {
		return errors.New("runsegment: child Run start persistence is unavailable")
	}
	record, err := childRunStartReservationRecord(reservation)
	if err != nil {
		return err
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		if _, err := e.childRunStarts.Conclude(
			ctx, record, sqlite.ChildRunStartConclusionAborted,
		); err != nil {
			return fmt.Errorf("runsegment: abort child Run start: %w", err)
		}
		return nil
	})
}

func childRunStartReservationRecord(
	reservation runs.ChildRunStartReservation,
) (sqlite.ChildRunStartReservationRecord, error) {
	if err := reservation.Validate(); err != nil {
		return sqlite.ChildRunStartReservationRecord{}, fmt.Errorf("runsegment: invalid child Run start reservation: %w", err)
	}
	payload, err := json.Marshal(childRunStartReservationPayload{
		ExecutorID:      reservation.ExecutorID,
		ParentMemberID:  reservation.Member.ParentID,
		SpawnCallID:     reservation.Member.SpawnCallID,
		RunID:           reservation.Binding.RunID,
		ParentRunID:     reservation.Binding.ParentRunID,
		SegmentID:       reservation.SegmentID,
		SpawnedByItemID: reservation.SpawnedByItemID,
		RootRunID:       reservation.RootRunID,
	})
	if err != nil {
		return sqlite.ChildRunStartReservationRecord{}, fmt.Errorf("runsegment: encode child Run start reservation: %w", err)
	}
	return sqlite.ChildRunStartReservationRecord{
		MemberID: reservation.Member.MemberID, SessionID: reservation.SessionID,
		Payload: payload, CreatedAt: reservation.StartedAt.UTC(),
	}, nil
}

// childRunStartReservationPayload is runsegment's canonical durable comparison
// payload. MemberID, SessionID, and StartedAt already have dedicated record
// columns; the remaining reservation facts stay adapter-owned instead of making
// an Application struct's Go layout an implicit storage wire.
type childRunStartReservationPayload struct {
	ExecutorID      string `json:"executorId"`
	ParentMemberID  string `json:"parentMemberId"`
	SpawnCallID     string `json:"spawnCallId"`
	RunID           string `json:"runId"`
	ParentRunID     string `json:"parentRunId"`
	SegmentID       string `json:"segmentId"`
	SpawnedByItemID string `json:"spawnedByItemId"`
	RootRunID       string `json:"rootRunId"`
}

func validateStartedChildOpening(
	reservation runs.ChildRunStartReservation,
	opening runs.OpeningCommit,
) error {
	if err := opening.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid started child opening: %w", err)
	}
	if opening.Admit == nil || opening.Resume != nil || opening.Admit.RunID != reservation.Binding.RunID ||
		opening.Admit.SessionID != reservation.SessionID ||
		opening.Admit.ParentRunID != reservation.Binding.ParentRunID ||
		opening.Admit.RootRunID != reservation.RootRunID ||
		opening.Admit.SpawnedByItemID != reservation.SpawnedByItemID ||
		opening.Admit.SegmentID != reservation.SegmentID ||
		!opening.Admit.CreatedAt.Equal(reservation.StartedAt) {
		return errors.New("runsegment: started child opening differs from its reservation")
	}
	return nil
}

// ClaimResume is the waiting-answer linearization point. Loading the exact
// checkpoint, consuming the exact Pending value, and deleting that checkpoint
// share one storage transaction. No executor operation occurs in this method.
func (e *Effects) ClaimResume(
	ctx context.Context,
	claim runs.ResumeClaimCommit,
) (runs.ClaimedResume, error) {
	if e.tx == nil {
		return runs.ClaimedResume{}, errors.New("runsegment: transactor is unavailable")
	}
	if e.resumeClaims == nil || e.executorCheckpoints == nil {
		return runs.ClaimedResume{}, errors.New("runsegment: resume-claim persistence is unavailable")
	}
	if err := claim.Validate(); err != nil {
		return runs.ClaimedResume{}, fmt.Errorf("runsegment: invalid resume claim: %w", err)
	}
	root, ok := claim.Expected.RootContinuation()
	if !ok {
		return runs.ClaimedResume{}, errors.New("runsegment: resume claim has no root continuation")
	}
	var checkpoint runs.ExecutorCheckpoint
	err := e.runInTx(ctx, func(ctx context.Context) error {
		loaded, err := e.executorCheckpoints.LoadCheckpoint(ctx, root.MemberID)
		if err != nil {
			return fmt.Errorf("runsegment: load claimed executor checkpoint: %w", err)
		}
		if err := loaded.ValidateOwnership(root.MemberID, claim.Expected.SessionID); err != nil {
			return err
		}
		if loaded.ModelSelection != root.ModelSelection || loaded.Limits != root.Limits ||
			loaded.Scope.GoalLeaseID != claim.Expected.GoalLeaseID {
			return fmt.Errorf("%w: claimed checkpoint policy differs from Pending", runs.ErrInvalidExecutorCheckpoint)
		}
		consumed, found, err := e.resumeClaims.ClaimResume(
			ctx, claim.Expected.SessionID, claim.Expected.RootRunID, claim.Answers, claim.ClaimedAt,
		)
		if err != nil {
			return fmt.Errorf("runsegment: consume resume Pending: %w", err)
		}
		if !found {
			return runs.ErrInterruptNotOpen
		}
		if !reflect.DeepEqual(consumed, claim.Expected) {
			return errors.New("runsegment: waiting hand-off changed before answer claim")
		}
		if err := e.executorCheckpoints.DeleteCheckpoints(
			ctx, claim.Expected.SessionID, []string{root.MemberID},
		); err != nil {
			return fmt.Errorf("runsegment: invalidate claimed executor checkpoint: %w", err)
		}
		checkpoint = loaded.Clone()
		return nil
	})
	if err != nil {
		return runs.ClaimedResume{}, err
	}
	return runs.ClaimedResume{
		Pending: claim.Expected, Answers: append([]runs.InterruptAnswer(nil), claim.Answers...),
		Checkpoint: checkpoint,
	}, nil
}

// CommitOpening accepts one segment atomically. A fresh segment admits its Run;
// a continuation resumes the existing Runs after a prior ResumeClaim consumed
// the waiting hand-off and invalidated its checkpoint. The
// opening transcript projections land in that same transaction, so Start cannot
// acknowledge a segment whose durable opening is missing.
func (e *Effects) CommitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	if err := opening.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid opening: %w", err)
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		return e.commitOpening(ctx, opening)
	})
}

// commitOpening assumes that the caller has validated opening and owns the
// transaction boundary. Keeping the persistence body separate prevents
// composite commits from depending on reentrant transaction implementations.
func (e *Effects) commitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	switch {
	case opening.Admit != nil:
		if err := e.admitOpening(ctx, opening); err != nil {
			return err
		}
	case opening.Resume != nil:
		if err := e.resumeTree(ctx, *opening.Resume); err != nil {
			return err
		}
	}
	for _, commit := range opening.Events {
		if err := e.applyCommit(ctx, commit); err != nil {
			return err
		}
	}
	return nil
}

func (e *Effects) admitOpening(ctx context.Context, opening runs.OpeningCommit) error {
	if opening.Admit == nil {
		return errors.New("runsegment: opening admission is required")
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	if opening.ScheduledSession != nil {
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
		if e.sessions == nil {
			return errors.New("runsegment: session persistence is unavailable")
		}
		if err := e.sessions.SetModel(
			ctx,
			opening.SessionModel.SessionID,
			opening.SessionModel.Model,
		); err != nil {
			return fmt.Errorf("runsegment: persist opening session model: %w", err)
		}
	}
	if opening.ScheduleFiring == "" {
		return nil
	}
	if e.scheduleFirings == nil {
		return errors.New("runsegment: schedule-firing persistence is unavailable")
	}
	if err := e.scheduleFirings.Accept(ctx, opening.ScheduleFiring, opening.Admit.RunID); err != nil {
		return fmt.Errorf("runsegment: accept scheduled occurrence: %w", err)
	}
	return nil
}

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// transcript item/run projections and the run-state transition in one
// transaction. A tree interruption is deliberately excluded: it must use
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
		if e.interrupts == nil {
			return errors.New("runsegment: interrupt persistence is unavailable")
		}
		if err := e.interrupts.Delete(ctx, commit.SessionID, commit.RunID); err != nil {
			return fmt.Errorf("runsegment: delete terminal interrupt for root Run %q: %w", commit.RunID, err)
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
	if err := barrier.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid tree barrier: %w", err)
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
		invocation, present := item.ToolInvocation()
		if !present || invocation.Offload == nil {
			continue
		}
		if err := e.toolResults.Discard(cleanupCtx, item.SessionID(), *invocation.Offload); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("runsegment: discard staged tool result %q: %w", invocation.Offload.ID, err))
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
	if err := e.applyModelInvocations(ctx, commit); err != nil {
		return err
	}
	if err := e.applyToolInvocations(ctx, commit); err != nil {
		return err
	}
	if err := e.applyProgress(ctx, commit); err != nil {
		return err
	}
	if err := e.applyState(ctx, commit); err != nil {
		return err
	}
	if commit.GoalRun != nil {
		if e.goalRuns == nil {
			return errors.New("runsegment: Goal Run persistence is unavailable")
		}
		if err := e.goalRuns.RecordRun(ctx, *commit.GoalRun); err != nil {
			return fmt.Errorf("runsegment: record Goal Run: %w", err)
		}
	}
	return nil
}

func (e *Effects) applyModelInvocations(ctx context.Context, commit runs.EventCommit) error {
	if len(commit.ModelInvocations) == 0 {
		return nil
	}
	if e.modelInvocations == nil {
		return errors.New("runsegment: model-invocation persistence is unavailable")
	}
	for _, invocation := range commit.ModelInvocations {
		var err error
		switch invocation.State {
		case runs.ModelInvocationStarted:
			err = e.modelInvocations.StartModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt,
			)
		case runs.ModelInvocationCompleted:
			err = e.modelInvocations.CompleteModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ModelInvocationFailed:
			err = e.modelInvocations.FailModelInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ModelInvocationUnknown:
			err = e.modelInvocations.MarkModelInvocationUnknown(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.StartedAt, invocation.FinishedAt,
			)
		default:
			err = fmt.Errorf("unsupported state %d", invocation.State)
		}
		if err != nil {
			return fmt.Errorf("runsegment: record model invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyToolInvocations(ctx context.Context, commit runs.EventCommit) error {
	if len(commit.ToolInvocations) == 0 {
		return nil
	}
	if e.toolInvocations == nil {
		return errors.New("runsegment: Tool-invocation persistence is unavailable")
	}
	for _, invocation := range commit.ToolInvocations {
		var err error
		switch invocation.State {
		case runs.ToolInvocationStarted:
			err = e.toolInvocations.StartToolInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID, invocation.StartedAt,
			)
		case runs.ToolInvocationCompleted:
			err = e.toolInvocations.CompleteToolInvocation(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID,
				invocation.StartedAt, invocation.FinishedAt,
			)
		case runs.ToolInvocationIncomplete:
			err = e.toolInvocations.MarkToolInvocationIncomplete(
				ctx, commit.SessionID, commit.RunID, invocation.SegmentID,
				invocation.CallID, invocation.ItemID,
				invocation.StartedAt, invocation.FinishedAt,
			)
		default:
			err = fmt.Errorf("unsupported state %d", invocation.State)
		}
		if err != nil {
			return fmt.Errorf("runsegment: record Tool invocation %q: %w", invocation.CallID, err)
		}
	}
	return nil
}

func (e *Effects) applyProgress(ctx context.Context, commit runs.EventCommit) error {
	if commit.Progress == nil {
		return nil
	}
	if e.runMetrics == nil {
		return errors.New("runsegment: Run-metrics persistence is unavailable")
	}
	if err := e.runMetrics.UpdateMetrics(
		ctx,
		commit.SessionID,
		commit.RunID,
		commit.Progress.SegmentID,
		commit.Progress.Metrics,
		commit.Progress.UpdatedAt,
	); err != nil {
		return fmt.Errorf("runsegment: update Run metrics: %w", err)
	}
	return nil
}

func (e *Effects) resumeTree(ctx context.Context, resume run.TreeResumeDraft) error {
	if err := resume.Validate(); err != nil {
		return fmt.Errorf("runsegment: invalid tree resume: %w", err)
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	if e.resumeClaims == nil {
		return errors.New("runsegment: resume-claim persistence is unavailable")
	}
	if err := e.resumeClaims.RequireResumeClaim(ctx, resume.SessionID, resume.RootRunID); err != nil {
		return fmt.Errorf("runsegment: require accepted answer claim: %w", err)
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

func (e *Effects) openInterrupt(ctx context.Context, p runs.Pending) error {
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
	invocation, present := item.ToolInvocation()
	if !present || invocation.Offload == nil {
		return nil
	}
	if invocation.Result == nil {
		return errors.New("runsegment: offloaded tool result is absent")
	}
	preview, ok := invocation.Result.String()
	if !ok {
		return errors.New("runsegment: offloaded tool result has no preview string")
	}
	if e.toolResults == nil {
		return errors.New("runsegment: tool-result persistence is unavailable")
	}
	if err := e.toolResults.Bind(ctx, item.SessionID(), item.ID(), preview, *invocation.Offload); err != nil {
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
func (e *Effects) finishedRun(ctx context.Context, commit runs.EventCommit) (run.Run, error) {
	if commit.Run == nil {
		return run.Run{}, errors.New("runsegment: terminal commit carries no run record")
	}
	record := *commit.Run
	if record.MessageMark() < 0 {
		if e.messages == nil {
			return run.Run{}, errors.New("runsegment: message persistence is unavailable")
		}
		mark, err := e.messages.Count(ctx, record.SessionID())
		if err != nil {
			return run.Run{}, fmt.Errorf("runsegment: resolve terminal message watermark: %w", err)
		}
		finalized, err := record.WithMessageMark(mark)
		if err != nil {
			return run.Run{}, fmt.Errorf("runsegment: apply terminal message watermark: %w", err)
		}
		record = finalized
	}
	return record, nil
}
