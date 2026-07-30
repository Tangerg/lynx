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

// CommitWaitingSubtreeCancellation commits the complete durable side of one
// prepared waiting-tree transformation. The Agent runtime remains frozen while
// this transaction runs; this adapter never commits or aborts that live
// mutation—it owns only application persistence.
func (e *Effects) CommitWaitingSubtreeCancellation(
	ctx context.Context,
	commit runs.WaitingSubtreeCancellationCommit,
) (runs.WaitingSubtreeCancellationResult, error) {
	if err := e.validateWaitingSubtreeCancellation(commit); err != nil {
		return runs.WaitingSubtreeCancellationResult{}, err
	}
	var (
		target transcript.Run
		root   transcript.Run
	)
	err := e.runInTx(ctx, func(ctx context.Context) error {
		pending, found, err := e.interrupts.Consume(ctx, commit.RootRunID)
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
		if err := commit.Checkpoint.PersistCheckpoint(ctx); err != nil {
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
			if err := e.interrupts.Put(ctx, *commit.RemainingPending); err != nil {
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
			if err := e.runState.Resume(
				ctx,
				commit.Resume.SessionID,
				draft,
				commit.Resume.ResumedAt,
			); err != nil {
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
			if event.State != runs.StateUnchanged {
				return errors.New("runsegment: waiting cancellation opening contains a lifecycle transition")
			}
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

func (e *Effects) validateWaitingSubtreeCancellation(
	commit runs.WaitingSubtreeCancellationCommit,
) error {
	switch {
	case e.tx == nil:
		return errors.New("runsegment: transactor is unavailable")
	case e.interrupts == nil:
		return errors.New("runsegment: interrupt persistence is unavailable")
	case e.runState == nil:
		return errors.New("runsegment: run-state persistence is unavailable")
	case e.itemReplacer == nil:
		return errors.New("runsegment: transcript Item replacement is unavailable")
	case commit.Checkpoint == nil:
		return errors.New("runsegment: waiting cancellation checkpoint is unavailable")
	case commit.RootRunID == "" || commit.TargetRunID == "" || commit.SessionID == "":
		return errors.New("runsegment: waiting cancellation identity is incomplete")
	case commit.RootRun.ID != commit.RootRunID ||
		commit.RootRun.SessionID != commit.SessionID ||
		!commit.RootRun.Lineage().IsRoot() ||
		commit.RootRun.State != execution.Interrupted:
		return errors.New("runsegment: waiting cancellation root snapshot is invalid")
	}
	if err := commit.ExpectedPending.Validate(); err != nil {
		return fmt.Errorf("runsegment: expected waiting cancellation interrupt: %w", err)
	}
	if commit.ExpectedPending.RootRunID != commit.RootRunID ||
		commit.ExpectedPending.SessionID != commit.SessionID {
		return errors.New("runsegment: expected waiting cancellation interrupt scope mismatch")
	}
	if (commit.RemainingPending == nil) == (commit.Resume == nil) {
		return errors.New("runsegment: waiting cancellation requires exactly one surviving disposition")
	}
	if commit.RemainingPending != nil {
		if err := commit.RemainingPending.Validate(); err != nil {
			return fmt.Errorf("runsegment: reduced waiting cancellation interrupt: %w", err)
		}
		if commit.RemainingPending.RootRunID != commit.RootRunID ||
			commit.RemainingPending.SessionID != commit.SessionID {
			return errors.New("runsegment: reduced waiting cancellation interrupt scope mismatch")
		}
		if len(commit.OpeningEvents) != 0 {
			return errors.New("runsegment: still-waiting cancellation carries Segment opening events")
		}
	} else if err := commit.Resume.Validate(); err != nil {
		return fmt.Errorf("runsegment: waiting cancellation tree resume: %w", err)
	} else if commit.Resume.RootRunID != commit.RootRunID ||
		commit.Resume.SessionID != commit.SessionID {
		return errors.New("runsegment: waiting cancellation tree resume scope mismatch")
	}
	if len(commit.TerminalRuns) == 0 {
		return errors.New("runsegment: waiting cancellation has no terminal Runs")
	}
	members := make([]execution.RunTreeMember, 0, len(commit.ExpectedPending.Continuations))
	continuationByRunID := make(
		map[string]interrupts.Continuation,
		len(commit.ExpectedPending.Continuations),
	)
	for _, continuation := range commit.ExpectedPending.Continuations {
		members = append(members, execution.RunTreeMember{
			RunID:   continuation.RunID,
			Lineage: continuation.Lineage,
		})
		continuationByRunID[continuation.RunID] = continuation
	}
	tree, err := execution.NewRunTree(commit.RootRunID, members)
	if err != nil {
		return fmt.Errorf("runsegment: waiting cancellation tree: %w", err)
	}
	canceledRunIDs, found := tree.SubtreePostorder(commit.TargetRunID)
	if !found {
		return fmt.Errorf(
			"runsegment: waiting cancellation target Run %q is outside tree %q",
			commit.TargetRunID,
			commit.RootRunID,
		)
	}
	if commit.TargetRunID == commit.RootRunID {
		return errors.New("runsegment: waiting subtree cancellation targets the root")
	}
	if len(commit.TerminalRuns) != len(canceledRunIDs) {
		return fmt.Errorf(
			"runsegment: waiting cancellation has %d terminal Runs, target subtree requires %d",
			len(commit.TerminalRuns),
			len(canceledRunIDs),
		)
	}
	seen := make(map[string]struct{}, len(commit.TerminalRuns))
	for index, run := range commit.TerminalRuns {
		expectedRunID := canceledRunIDs[index]
		continuation := continuationByRunID[expectedRunID]
		switch {
		case run.ID != expectedRunID:
			return fmt.Errorf(
				"runsegment: canceled Run[%d] is %q, canonical target postorder requires %q",
				index,
				run.ID,
				expectedRunID,
			)
		case run.SessionID != commit.SessionID:
			return fmt.Errorf("runsegment: canceled Run[%d] session mismatch", index)
		case run.Lineage() != continuation.Lineage:
			return fmt.Errorf("runsegment: canceled Run[%d] lineage mismatch", index)
		case run.State != execution.Canceled ||
			run.Outcome == nil ||
			*run.Outcome != execution.OutcomeCanceled:
			return fmt.Errorf("runsegment: canceled Run[%d] is not canceled", index)
		case run.ID == commit.RootRunID:
			return errors.New("runsegment: waiting child cancellation terminalizes the root")
		}
		if _, duplicate := seen[run.ID]; duplicate {
			return fmt.Errorf("runsegment: waiting cancellation repeats Run %q", run.ID)
		}
		seen[run.ID] = struct{}{}
	}

	canceledSet := make(map[string]struct{}, len(canceledRunIDs))
	canceledProcessSet := make(map[string]struct{}, len(canceledRunIDs))
	for _, runID := range canceledRunIDs {
		canceledSet[runID] = struct{}{}
		canceledProcessSet[continuationByRunID[runID].ProcessID] = struct{}{}
	}
	var survivingRunIDs []string
	for _, runID := range tree.Postorder() {
		if _, canceled := canceledSet[runID]; !canceled {
			survivingRunIDs = append(survivingRunIDs, runID)
		}
	}
	var dispositionRunIDs []string
	if commit.RemainingPending != nil {
		var survivingInterruptIndexes []int
		for index, binding := range commit.ExpectedPending.Suspensions {
			if _, canceled := canceledProcessSet[binding.ProcessID]; !canceled {
				survivingInterruptIndexes = append(survivingInterruptIndexes, index)
			}
		}
		if len(commit.RemainingPending.Suspensions) != len(survivingInterruptIndexes) {
			return fmt.Errorf(
				"runsegment: reduced waiting cancellation has %d suspensions, surviving tree requires %d",
				len(commit.RemainingPending.Suspensions),
				len(survivingInterruptIndexes),
			)
		}
		for index, expectedIndex := range survivingInterruptIndexes {
			if commit.RemainingPending.Suspensions[index] !=
				commit.ExpectedPending.Suspensions[expectedIndex] ||
				!sameInterruptSnapshot(
					commit.RemainingPending.Interrupts[index],
					commit.ExpectedPending.Interrupts[expectedIndex],
				) {
				return fmt.Errorf(
					"runsegment: reduced waiting cancellation changed surviving suspension[%d]",
					index,
				)
			}
		}
		for _, continuation := range commit.RemainingPending.Continuations {
			dispositionRunIDs = append(dispositionRunIDs, continuation.RunID)
		}
		if commit.RemainingPending.TurnID != commit.ExpectedPending.TurnID ||
			!commit.RemainingPending.CreatedAt.Equal(commit.ExpectedPending.CreatedAt) ||
			!reflect.DeepEqual(
				normalizeProtocolProfile(commit.RemainingPending.ProtocolProfile),
				normalizeProtocolProfile(commit.ExpectedPending.ProtocolProfile),
			) {
			return errors.New("runsegment: reduced waiting cancellation changed immutable pending facts")
		}
	} else {
		for _, binding := range commit.ExpectedPending.Suspensions {
			if _, canceled := canceledProcessSet[binding.ProcessID]; !canceled {
				return fmt.Errorf(
					"runsegment: waiting cancellation resumes while process %q suspension %q survives",
					binding.ProcessID,
					binding.SuspensionID,
				)
			}
		}
		for _, draft := range commit.Resume.Runs {
			dispositionRunIDs = append(dispositionRunIDs, draft.RunID)
		}
	}
	if !slices.Equal(dispositionRunIDs, survivingRunIDs) {
		return fmt.Errorf(
			"runsegment: waiting cancellation disposition Runs %v, surviving tree requires %v",
			dispositionRunIDs,
			survivingRunIDs,
		)
	}
	survivingSet := make(map[string]struct{}, len(survivingRunIDs))
	for _, runID := range survivingRunIDs {
		survivingSet[runID] = struct{}{}
	}
	for index, event := range commit.OpeningEvents {
		if event.State != runs.StateUnchanged ||
			event.Run != nil ||
			event.GoalTurn != nil ||
			event.SessionID != commit.SessionID {
			return fmt.Errorf(
				"runsegment: waiting cancellation opening event[%d] is not an item-only projection",
				index,
			)
		}
		if _, survives := survivingSet[event.RunID]; !survives {
			return fmt.Errorf(
				"runsegment: waiting cancellation opening event[%d] names removed Run %q",
				index,
				event.RunID,
			)
		}
		if len(event.Items) == 0 {
			return fmt.Errorf(
				"runsegment: waiting cancellation opening event[%d] has no Items",
				index,
			)
		}
		for _, item := range event.Items {
			if item.SessionID != event.SessionID || item.RunID != event.RunID {
				return fmt.Errorf(
					"runsegment: waiting cancellation opening event[%d] Item %q scope mismatch",
					index,
					item.ID,
				)
			}
		}
	}

	expectedItem, replacement := commit.ParentItem.Expected, commit.ParentItem.Replacement
	targetContinuation := continuationByRunID[commit.TargetRunID]
	if expectedItem.ID == "" ||
		expectedItem.ID != replacement.ID ||
		expectedItem.ID != targetContinuation.Lineage.SpawnedByItemID ||
		expectedItem.SessionID != commit.SessionID ||
		replacement.SessionID != commit.SessionID ||
		expectedItem.RunID != replacement.RunID ||
		expectedItem.RunID != targetContinuation.Lineage.ParentRunID {
		return errors.New("runsegment: waiting cancellation parent Item identity mismatch")
	}
	replacementWithoutProblem := replacement
	replacementWithoutProblem.Error = nil
	if expectedItem.Kind != transcript.ToolCall ||
		expectedItem.Tool == nil ||
		expectedItem.Status != transcript.ItemIncomplete ||
		expectedItem.Error != nil ||
		!sameItemSnapshot(expectedItem, replacementWithoutProblem) {
		return errors.New(
			"runsegment: waiting cancellation replacement changes parent Item facts beyond its problem",
		)
	}
	if replacement.Status != transcript.ItemIncomplete ||
		replacement.Error == nil ||
		replacement.Error.Kind != transcript.ChildRunCanceledProblem ||
		replacement.Error.Scope != transcript.ToolProblem {
		return errors.New("runsegment: waiting cancellation parent Item lacks child_run_canceled")
	}
	if commit.RemainingPending != nil {
		for _, actual := range commit.RemainingPending.Continuations {
			expected := continuationByRunID[actual.RunID]
			if actual.RunID == targetContinuation.Lineage.ParentRunID {
				var matched []interrupts.DrainedTool
				expected.DrainedTools = slices.DeleteFunc(
					slices.Clone(expected.DrainedTools),
					func(candidate interrupts.DrainedTool) bool {
						if candidate.ItemID != expectedItem.ID {
							return false
						}
						matched = append(matched, candidate)
						return true
					},
				)
				if len(matched) != 1 {
					return fmt.Errorf(
						"runsegment: parent continuation has %d drained tools for spawning Item %q",
						len(matched),
						expectedItem.ID,
					)
				}
				settled := matched[0]
				expected.CommittedTools = append(
					slices.Clone(expected.CommittedTools),
					interrupts.CommittedTool{
						ItemID:    settled.ItemID,
						CallID:    settled.CallID,
						Name:      settled.Name,
						Arguments: settled.Arguments,
						Problem:   *replacement.Error,
					},
				)
			}
			if !sameContinuationSnapshot(actual, expected) {
				return fmt.Errorf(
					"runsegment: reduced waiting cancellation changed continuation for Run %q",
					actual.RunID,
				)
			}
		}
	}
	return nil
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

// samePendingSnapshot compares the command's frozen optimistic-lock value with
// the row claimed inside the transaction. SQLite round-trips zero-length nested
// slices as allocated empty slices, while in-memory reducers commonly leave
// them nil; those are the same domain value and must not create a false conflict.
// Times are compared after removing monotonic/location representation details.
func samePendingSnapshot(left, right interrupts.Pending) bool {
	return reflect.DeepEqual(
		normalizePendingSnapshot(left),
		normalizePendingSnapshot(right),
	)
}

func normalizePendingSnapshot(pending interrupts.Pending) interrupts.Pending {
	pending.Interrupts = slices.Clone(pending.Interrupts)
	pending.Suspensions = slices.Clone(pending.Suspensions)
	pending.Continuations = slices.Clone(pending.Continuations)
	pending.CreatedAt = timeFromUnixNano(pending.CreatedAt)
	pending.ProtocolProfile = normalizeProtocolProfile(pending.ProtocolProfile)
	for index := range pending.Continuations {
		pending.Continuations[index] = normalizeContinuationSnapshot(
			pending.Continuations[index],
		)
	}
	for index := range pending.Interrupts {
		source := pending.Interrupts[index].Question
		if source == nil {
			continue
		}
		questionValue := *source
		questionValue.Fields = slices.Clone(source.Fields)
		question := &questionValue
		pending.Interrupts[index].Question = question
		if len(question.Fields) == 0 {
			question.Fields = nil
			continue
		}
		for fieldIndex := range question.Fields {
			if len(question.Fields[fieldIndex].Options) == 0 {
				question.Fields[fieldIndex].Options = nil
			}
		}
	}
	return pending
}

func sameContinuationSnapshot(left, right interrupts.Continuation) bool {
	return reflect.DeepEqual(
		normalizeContinuationSnapshot(left),
		normalizeContinuationSnapshot(right),
	)
}

func normalizeContinuationSnapshot(
	continuation interrupts.Continuation,
) interrupts.Continuation {
	continuation.RunCreatedAt = timeFromUnixNano(continuation.RunCreatedAt)
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

func normalizeProtocolProfile(profile execution.RunProtocolProfile) execution.RunProtocolProfile {
	profile = profile.Normalized()
	if len(profile.RequiredFeatures) == 0 {
		profile.RequiredFeatures = nil
	}
	if len(profile.InterruptKinds) == 0 {
		profile.InterruptKinds = nil
	}
	return profile
}

func sameItemSnapshot(left, right transcript.Item) bool {
	return reflect.DeepEqual(
		normalizeItemSnapshot(left),
		normalizeItemSnapshot(right),
	)
}

func sameInterruptSnapshot(left, right transcript.Interrupt) bool {
	pending := normalizePendingSnapshot(interrupts.Pending{
		Interrupts: []transcript.Interrupt{left, right},
	})
	return reflect.DeepEqual(pending.Interrupts[0], pending.Interrupts[1])
}

func normalizeItemSnapshot(item transcript.Item) transcript.Item {
	item.CreatedAt = timeFromUnixNano(item.CreatedAt)
	if len(item.Content) == 0 {
		item.Content = nil
	}
	if len(item.Steps) == 0 {
		item.Steps = nil
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
