package runsegment

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
		if !samePendingSnapshot(pending, commit.ExpectedPending) {
			return fmt.Errorf(
				"%w: waiting cancellation interrupt for root Run %q changed after preparation",
				runs.ErrSessionBusy,
				commit.RootRunID,
			)
		}
		if err := e.executorCheckpoints.SaveCheckpoint(ctx, commit.Checkpoint); err != nil {
			return fmt.Errorf(
				"runsegment: persist checkpoint for waiting child Run %q in root Run %q: %w",
				commit.TargetRunID,
				commit.RootRunID,
				err,
			)
		}
		if err := e.itemReplacer.ReplaceItem(ctx, commit.ParentItem.Expected, commit.ParentItem.Replacement); err != nil {
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

		terminalByID := make(map[string]transcript.Run, len(commit.TerminalRuns))
		for _, planned := range commit.TerminalRuns {
			finalized, err := e.finishedRun(ctx, runs.EventCommit{
				RunID:     planned.ID,
				SessionID: planned.SessionID,
				State:     runs.StateTerminalize,
				Outcome:   execution.OutcomeCanceled,
				Run:       &planned,
			})
			if err != nil {
				return fmt.Errorf("runsegment: finalize canceled Run %q: %w", planned.ID, err)
			}
			if err := e.runState.Terminalize(ctx, finalized); err != nil {
				return fmt.Errorf("runsegment: terminalize canceled Run %q: %w", planned.ID, err)
			}
			terminalByID[planned.ID] = finalized
		}
		var targetFound bool
		target, targetFound = terminalByID[commit.TargetRunID]
		if !targetFound {
			return fmt.Errorf("runsegment: target Run %q was not terminalized", commit.TargetRunID)
		}

		root = commit.RootRun
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
				root.State = execution.Running
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

func directInterrupts(pending interrupts.Pending, runID string) []transcript.Interrupt {
	out := make([]transcript.Interrupt, 0, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		if interrupt.RunID == runID {
			out = append(out, interrupt)
		}
	}
	return out
}

// samePendingSnapshot compares the frozen optimistic-lock value with the row
// claimed inside the transaction. SQLite may round-trip equivalent empty slices
// and time representations differently, so those storage forms are normalized.
func samePendingSnapshot(left, right interrupts.Pending) bool {
	return reflect.DeepEqual(normalizePendingSnapshot(left), normalizePendingSnapshot(right))
}

func normalizePendingSnapshot(pending interrupts.Pending) interrupts.Pending {
	pending.Interrupts = slices.Clone(pending.Interrupts)
	pending.Suspensions = slices.Clone(pending.Suspensions)
	pending.Continuations = slices.Clone(pending.Continuations)
	pending.CreatedAt = timeFromUnixNano(pending.CreatedAt)
	pending.Capabilities = normalizeCapabilities(pending.Capabilities)
	for index := range pending.Continuations {
		pending.Continuations[index] = normalizeContinuationSnapshot(pending.Continuations[index])
	}
	for index := range pending.Interrupts {
		pending.Interrupts[index].ItemOccurredAt = timeFromUnixNano(pending.Interrupts[index].ItemOccurredAt)
		if pending.Interrupts[index].Question == nil {
			continue
		}
		question := *pending.Interrupts[index].Question
		question.Fields = slices.Clone(question.Fields)
		for fieldIndex := range question.Fields {
			if len(question.Fields[fieldIndex].Options) == 0 {
				question.Fields[fieldIndex].Options = nil
			}
		}
		if len(question.Fields) == 0 {
			question.Fields = nil
		}
		pending.Interrupts[index].Question = &question
	}
	return pending
}

func normalizeContinuationSnapshot(continuation interrupts.Continuation) interrupts.Continuation {
	continuation.RunCreatedAt = timeFromUnixNano(continuation.RunCreatedAt)
	for index := range continuation.DrainedTools {
		continuation.DrainedTools[index].ItemOccurredAt = timeFromUnixNano(continuation.DrainedTools[index].ItemOccurredAt)
	}
	if len(continuation.DrainedTools) == 0 {
		continuation.DrainedTools = nil
	}
	if len(continuation.CommittedTools) == 0 {
		continuation.CommittedTools = nil
	}
	if continuation.Metrics.Usage != nil {
		usage := *continuation.Metrics.Usage
		if len(usage.ByModel) == 0 {
			usage.ByModel = nil
		}
		continuation.Metrics.Usage = &usage
	}
	return continuation
}

func normalizeCapabilities(capabilities execution.RunCapabilities) execution.RunCapabilities {
	capabilities = capabilities.Normalized()
	if len(capabilities.InterruptKinds) == 0 {
		capabilities.InterruptKinds = nil
	}
	return capabilities
}

func sameItemSnapshot(left, right transcript.Item) bool {
	return reflect.DeepEqual(normalizeItemSnapshot(left), normalizeItemSnapshot(right))
}

func normalizeItemSnapshot(item transcript.Item) transcript.Item {
	item.OccurredAt = timeFromUnixNano(item.OccurredAt)
	if len(item.Content) == 0 {
		item.Content = nil
	}
	if item.Question != nil {
		question := *item.Question
		question.Fields = slices.Clone(question.Fields)
		for index := range question.Fields {
			if len(question.Fields[index].Options) == 0 {
				question.Fields[index].Options = nil
			}
		}
		if len(question.Fields) == 0 {
			question.Fields = nil
		}
		item.Question = &question
	}
	return item
}

func timeFromUnixNano(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Unix(0, value.UnixNano()).UTC()
}
