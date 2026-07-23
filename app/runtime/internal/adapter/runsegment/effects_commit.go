package runsegment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
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
	if (opening.Admit == nil) == (opening.Resume == nil) {
		return errors.New("runsegment: opening requires exactly one admission action")
	}
	if len(opening.Events) == 0 {
		return errors.New("runsegment: opening requires a durable projection")
	}
	return e.runInTx(ctx, func(ctx context.Context) error {
		switch {
		case opening.Admit != nil:
			if e.runState == nil {
				return errors.New("runsegment: run-state persistence is unavailable")
			}
			if err := e.runState.Admit(ctx, *opening.Admit); err != nil {
				return err
			}
		case opening.Resume != nil:
			if err := e.consumeResume(ctx, *opening.Resume); err != nil {
				return err
			}
		}
		for _, commit := range opening.Events {
			if commit.Interrupt != nil || commit.State != runs.StateUnchanged {
				return errors.New("runsegment: opening commit contains a lifecycle transition")
			}
			if len(commit.Items) == 0 && commit.Run == nil {
				return errors.New("runsegment: opening commit has no durable projection")
			}
			if err := e.applyCommit(ctx, commit, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// CommitEvent applies one run event's durable parts atomically (§8.3/§8.4): the
// open-interrupt record, transcript item/run projections, and the run-state
// transition, all in one transaction. The interrupt's recoverable process id is
// resolved from the live turn BEFORE the transaction opens (an in-memory lookup,
// not a DB read) and its absence fails the commit — a park with no recoverable
// process is not resumable. A terminal run's message watermark is resolved inside
// the transaction so it is consistent with the state it terminalizes.
func (e *Effects) CommitEvent(ctx context.Context, commit runs.EventCommit) error {
	if e.tx == nil {
		return errors.New("runsegment: transactor is unavailable")
	}
	var pending *interrupts.Pending
	if commit.Interrupt != nil {
		p := *commit.Interrupt
		procID, err := e.interruptProcessID(ctx, p)
		if err != nil {
			return e.compensateFailedCommit(ctx, commit, err)
		}
		p.ProcessID = procID
		pending = &p
	}
	err := e.runInTx(ctx, func(ctx context.Context) error { return e.applyCommit(ctx, commit, pending) })
	if err != nil {
		return e.compensateFailedCommit(ctx, commit, err)
	}
	return nil
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

func (e *Effects) applyCommit(ctx context.Context, commit runs.EventCommit, pending *interrupts.Pending) error {
	if pending != nil {
		if err := e.putInterrupt(ctx, *pending); err != nil {
			return err
		}
	}
	for _, item := range commit.Items {
		if err := e.appendItem(ctx, item); err != nil {
			return err
		}
	}
	if commit.Run != nil {
		if err := e.putRun(ctx, *commit.Run, commit.State == runs.StateTerminalize); err != nil {
			return err
		}
	}
	return e.applyState(ctx, commit)
}

func (e *Effects) consumeResume(ctx context.Context, resume execution.ResumeDraft) error {
	if e.interrupts == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	pending, ok, err := e.interrupts.Consume(ctx, resume.RunID)
	if err != nil {
		return fmt.Errorf("runsegment: consume resume interrupt: %w", err)
	}
	if !ok {
		return errors.New("runsegment: resume interrupt is no longer open")
	}
	if pending.SessionID != resume.SessionID {
		return fmt.Errorf("runsegment: resume interrupt session mismatch: got %q want %q", pending.SessionID, resume.SessionID)
	}
	if e.runState == nil {
		return errors.New("runsegment: run-state persistence is unavailable")
	}
	if err := e.runState.Resume(ctx, resume); err != nil {
		return fmt.Errorf("runsegment: resume run state: %w", err)
	}
	return nil
}

func (e *Effects) runInTx(ctx context.Context, fn func(context.Context) error) error {
	return e.tx(ctx, fn)
}

func (e *Effects) interruptProcessID(ctx context.Context, p interrupts.Pending) (string, error) {
	if e.processes == nil {
		return "", errors.New("runsegment: process lookup is unavailable")
	}
	// Rebuild the executor's turn handle from the persisted coordinates — the
	// dispatcher keys the live turn by session + turn id, and the domain record
	// carries both, so runsegment needs no adapter handle in the commit value.
	procID, err := e.processes.ProcessID(ctx, turn.TurnHandle{SessionID: p.SessionID, TurnID: p.TurnID})
	if err != nil {
		return "", fmt.Errorf("runsegment: resolve interrupt process: %w", err)
	}
	if procID == "" {
		return "", errors.New("runsegment: interrupt process id is empty")
	}
	return procID, nil
}

func (e *Effects) putInterrupt(ctx context.Context, p interrupts.Pending) error {
	if e.interrupts == nil {
		return errors.New("runsegment: interrupt persistence is unavailable")
	}
	if err := e.interrupts.Put(ctx, p); err != nil {
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

// putRun upserts a transcript run, resolving the terminal message watermark
// inside the caller's transaction — the mark the rollback / fork boundary math
// truncates the chat log to. The message log is in its terminal post-maintenance
// (post-compaction) shape by the time the terminal event reaches here.
func (e *Effects) putRun(ctx context.Context, run transcript.Run, terminal bool) error {
	if e.transcript == nil {
		return errors.New("runsegment: transcript persistence is unavailable")
	}
	if terminal && run.MessageMark < 0 {
		if e.messages == nil {
			return errors.New("runsegment: message persistence is unavailable")
		}
		mark, err := e.messages.Count(ctx, run.SessionID)
		if err != nil {
			return fmt.Errorf("runsegment: resolve terminal message watermark: %w", err)
		}
		run.MessageMark = mark
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = time.Now().UTC()
	}
	return e.transcript.PutRun(ctx, run)
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
		return e.runState.Suspend(ctx, commit.SessionID, commit.RunID)
	case runs.StateTerminalize:
		return e.runState.Terminalize(ctx, commit.SessionID, commit.RunID, commit.Outcome)
	default:
		return fmt.Errorf("runsegment: unknown run state change %d", commit.State)
	}
}
