package runs

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type waitingCancellationValidation struct {
	commit              WaitingSubtreeCancellationCommit
	continuationByRunID map[string]Continuation
	canceledRunIDs      []string
	canceledProcessIDs  map[string]struct{}
	survivingRunIDs     []string
	terminalRunIDs      map[string]struct{}
	finishedAtByRunID   map[string]time.Time
}

// Validate proves that the write-set is exactly the canonical transformation
// of ExpectedPending after TargetRunID's subtree is removed. This is application
// policy: persistence only claims the frozen Pending snapshot and writes these
// already-validated facts atomically.
func (c WaitingSubtreeCancellationCommit) Validate() error {
	validation, err := newWaitingCancellationValidation(c)
	if err != nil {
		return err
	}
	if err := validation.validateTerminalRuns(); err != nil {
		return err
	}
	if err := validation.validateTerminalItems(); err != nil {
		return err
	}
	if err := validation.validateParentItem(); err != nil {
		return err
	}
	if err := validation.validateDisposition(); err != nil {
		return err
	}
	if err := validation.validateOpeningEvents(); err != nil {
		return err
	}
	return nil
}

func newWaitingCancellationValidation(
	c WaitingSubtreeCancellationCommit,
) (waitingCancellationValidation, error) {
	if strings.TrimSpace(c.RootRunID) == "" ||
		strings.TrimSpace(c.TargetRunID) == "" ||
		strings.TrimSpace(c.SessionID) == "" {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation identity is incomplete")
	}
	if c.RootRun.ID != c.RootRunID ||
		c.RootRun.SessionID != c.SessionID ||
		!c.RootRun.Lineage().IsRoot() ||
		c.RootRun.State != rundomain.Waiting {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation root snapshot is invalid")
	}
	if err := c.ExpectedPending.Validate(); err != nil {
		return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation expected Pending: %w", err)
	}
	if c.ExpectedPending.RootRunID != c.RootRunID || c.ExpectedPending.SessionID != c.SessionID {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation expected Pending scope mismatch")
	}
	rootContinuation, ok := c.ExpectedPending.RootContinuation()
	if !ok {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation expected Pending has no root continuation")
	}
	if c.RootRun.GoalLeaseID != c.ExpectedPending.GoalLeaseID {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation root Run goal lease differs from Pending")
	}
	if !c.RootRun.Capabilities.Equal(c.ExpectedPending.Capabilities) {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation root Run capabilities differ from Pending")
	}
	if err := validateWaitingRunContinuation(c.RootRun, rootContinuation); err != nil {
		return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation root Run: %w", err)
	}
	if err := c.Checkpoint.ValidateOwnership(rootContinuation.ProcessID, c.SessionID); err != nil {
		return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation checkpoint ownership: %w", err)
	}
	if c.Checkpoint.Scope.GoalLeaseID != c.ExpectedPending.GoalLeaseID ||
		c.Checkpoint.ModelSelection != rootContinuation.ModelSelection ||
		c.Checkpoint.Limits != rootContinuation.Limits {
		return waitingCancellationValidation{}, fmt.Errorf(
			"runs: waiting cancellation checkpoint differs from root continuation: %w",
			ErrInvalidExecutorCheckpoint,
		)
	}
	if (c.RemainingPending == nil) == (c.Resume == nil) {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation requires exactly one surviving disposition")
	}
	if c.RemainingPending != nil {
		if err := c.RemainingPending.Validate(); err != nil {
			return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation reduced Pending: %w", err)
		}
		if c.RemainingPending.RootRunID != c.RootRunID || c.RemainingPending.SessionID != c.SessionID {
			return waitingCancellationValidation{}, errors.New("runs: waiting cancellation reduced Pending scope mismatch")
		}
		if len(c.OpeningEvents) != 0 {
			return waitingCancellationValidation{}, errors.New("runs: waiting cancellation still parked carries opening events")
		}
	} else {
		if err := c.Resume.Validate(); err != nil {
			return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation Resume: %w", err)
		}
		if c.Resume.RootRunID != c.RootRunID || c.Resume.SessionID != c.SessionID {
			return waitingCancellationValidation{}, errors.New("runs: waiting cancellation Resume scope mismatch")
		}
	}

	members := make([]rundomain.RunTreeMember, 0, len(c.ExpectedPending.Continuations))
	continuationByRunID := make(map[string]Continuation, len(c.ExpectedPending.Continuations))
	for _, continuation := range c.ExpectedPending.Continuations {
		members = append(members, rundomain.RunTreeMember{RunID: continuation.RunID, Lineage: continuation.Lineage})
		continuationByRunID[continuation.RunID] = continuation
	}
	tree, err := rundomain.NewRunTree(c.RootRunID, members)
	if err != nil {
		return waitingCancellationValidation{}, fmt.Errorf("runs: waiting cancellation tree: %w", err)
	}
	canceledRunIDs, found := tree.SubtreePostorder(c.TargetRunID)
	if !found || c.TargetRunID == c.RootRunID {
		return waitingCancellationValidation{}, errors.New("runs: waiting cancellation target is not a child in the pending tree")
	}
	canceledRunSet := make(map[string]struct{}, len(canceledRunIDs))
	canceledProcessIDs := make(map[string]struct{}, len(canceledRunIDs))
	for _, runID := range canceledRunIDs {
		canceledRunSet[runID] = struct{}{}
		canceledProcessIDs[continuationByRunID[runID].ProcessID] = struct{}{}
	}
	var survivingRunIDs []string
	for _, runID := range tree.Postorder() {
		if _, canceled := canceledRunSet[runID]; !canceled {
			survivingRunIDs = append(survivingRunIDs, runID)
		}
	}
	return waitingCancellationValidation{
		commit:              c,
		continuationByRunID: continuationByRunID,
		canceledRunIDs:      canceledRunIDs,
		canceledProcessIDs:  canceledProcessIDs,
		survivingRunIDs:     survivingRunIDs,
		terminalRunIDs:      make(map[string]struct{}, len(c.TerminalRuns)),
		finishedAtByRunID:   make(map[string]time.Time, len(c.TerminalRuns)),
	}, nil
}

func (v *waitingCancellationValidation) validateTerminalRuns() error {
	c := v.commit
	if len(c.TerminalRuns) != len(v.canceledRunIDs) {
		return fmt.Errorf(
			"runs: waiting cancellation has %d terminal Runs, target subtree requires %d",
			len(c.TerminalRuns),
			len(v.canceledRunIDs),
		)
	}
	for index, run := range c.TerminalRuns {
		expectedRunID := v.canceledRunIDs[index]
		continuation := v.continuationByRunID[expectedRunID]
		switch {
		case run.ID != expectedRunID:
			return fmt.Errorf("runs: waiting cancellation Run[%d] is %q, want %q", index, run.ID, expectedRunID)
		case run.SessionID != c.SessionID:
			return fmt.Errorf("runs: waiting cancellation Run[%d] Session mismatch", index)
		case run.Lineage() != continuation.Lineage:
			return fmt.Errorf("runs: waiting cancellation Run[%d] lineage mismatch", index)
		case run.ModelSelection != continuation.ModelSelection:
			return fmt.Errorf("runs: waiting cancellation Run[%d] model mismatch", index)
		case !run.Metrics.Equal(continuation.Metrics):
			return fmt.Errorf("runs: waiting cancellation Run[%d] metrics mismatch", index)
		case run.Limits != continuation.Limits:
			return fmt.Errorf("runs: waiting cancellation Run[%d] limits mismatch", index)
		case !run.CreatedAt.Equal(continuation.RunCreatedAt):
			return fmt.Errorf("runs: waiting cancellation Run[%d] creation time mismatch", index)
		case !run.Capabilities.Equal(c.ExpectedPending.Capabilities):
			return fmt.Errorf("runs: waiting cancellation Run[%d] capabilities mismatch", index)
		case run.GoalLeaseID != "":
			return fmt.Errorf("runs: waiting cancellation child Run[%d] carries a root Goal lease", index)
		case run.State != rundomain.Canceled || run.Outcome == nil || *run.Outcome != rundomain.OutcomeCanceled:
			return fmt.Errorf("runs: waiting cancellation Run[%d] is not canceled", index)
		}
		if _, duplicate := v.terminalRunIDs[run.ID]; duplicate {
			return fmt.Errorf("runs: waiting cancellation repeats Run %q", run.ID)
		}
		v.terminalRunIDs[run.ID] = struct{}{}
		v.finishedAtByRunID[run.ID] = run.FinishedAt
	}
	return nil
}

func (v waitingCancellationValidation) validateTerminalItems() error {
	c := v.commit
	expectedByItemID := make(map[string]transcript.Interrupt)
	for _, interrupt := range c.ExpectedPending.Interrupts {
		if _, terminal := v.terminalRunIDs[interrupt.RunID]; terminal {
			expectedByItemID[interrupt.ItemID] = interrupt
		}
	}
	if len(c.TerminalItems) != len(expectedByItemID) {
		return fmt.Errorf("runs: waiting cancellation has %d terminal Items, want %d", len(c.TerminalItems), len(expectedByItemID))
	}
	seen := make(map[string]struct{}, len(c.TerminalItems))
	for index, replacement := range c.TerminalItems {
		interrupt, expected := expectedByItemID[replacement.Expected.ID]
		if !expected || replacement.Expected.SessionID != c.SessionID ||
			replacement.Expected.RunID != interrupt.RunID || replacement.Expected.Status != transcript.ItemRunning {
			return fmt.Errorf("runs: waiting cancellation terminal Item[%d] is outside canceled interrupts", index)
		}
		if _, duplicate := seen[replacement.Expected.ID]; duplicate {
			return fmt.Errorf("runs: waiting cancellation repeats terminal Item %q", replacement.Expected.ID)
		}
		seen[replacement.Expected.ID] = struct{}{}
		if err := validateTerminalItemReplacement(interrupt, replacement, v.finishedAtByRunID[interrupt.RunID]); err != nil {
			return fmt.Errorf("runs: waiting cancellation terminal Item %q: %w", replacement.Expected.ID, err)
		}
	}
	return nil
}

func validateTerminalItemReplacement(
	request transcript.Interrupt,
	replacement ItemReplacement,
	finishedAt time.Time,
) error {
	switch request.Kind {
	case interrupt.Question:
		if replacement.Expected.Kind != transcript.QuestionItem ||
			replacement.Expected.Question == nil || request.Question == nil ||
			!reflect.DeepEqual(replacement.Expected.Question, request.Question) {
			return errors.New("question differs from its interrupt")
		}
	case interrupt.Approval:
		if replacement.Expected.Kind != transcript.ToolCall || replacement.Expected.Tool == nil ||
			request.Approval == nil || !reflect.DeepEqual(*replacement.Expected.Tool, request.Approval.Tool) {
			return errors.New("approval tool differs from its interrupt")
		}
	default:
		return fmt.Errorf("unsupported interrupt kind %s", request.Kind)
	}
	expected := replacement.Expected
	expected.Status = transcript.ItemIncomplete
	if expected.Kind == transcript.ToolCall {
		expected.FinishedAt = finishedAt
		expected.Error = replacement.Replacement.Error
		if expected.Error == nil || expected.Error.Kind != transcript.ToolFailedProblem || expected.Error.Scope != transcript.ToolProblem {
			return errors.New("tool replacement has an invalid problem")
		}
	}
	if !reflect.DeepEqual(expected, replacement.Replacement) {
		return errors.New("replacement changes facts beyond terminal status")
	}
	return nil
}

func (v waitingCancellationValidation) validateDisposition() error {
	c := v.commit
	var dispositionRunIDs []string
	if c.RemainingPending != nil {
		var survivingSuspensionIndexes []int
		for index, binding := range c.ExpectedPending.Suspensions {
			if _, canceled := v.canceledProcessIDs[binding.ProcessID]; !canceled {
				survivingSuspensionIndexes = append(survivingSuspensionIndexes, index)
			}
		}
		if len(c.RemainingPending.Suspensions) != len(survivingSuspensionIndexes) {
			return errors.New("runs: waiting cancellation reduced Pending has the wrong suspension set")
		}
		for index, expectedIndex := range survivingSuspensionIndexes {
			if c.RemainingPending.Suspensions[index] != c.ExpectedPending.Suspensions[expectedIndex] ||
				!sameInterruptValue(c.RemainingPending.Interrupts[index], c.ExpectedPending.Interrupts[expectedIndex]) {
				return fmt.Errorf("runs: waiting cancellation changed surviving suspension[%d]", index)
			}
		}
		for _, continuation := range c.RemainingPending.Continuations {
			dispositionRunIDs = append(dispositionRunIDs, continuation.RunID)
		}
		if c.RemainingPending.ExecutorID != c.ExpectedPending.ExecutorID ||
			c.RemainingPending.GoalLeaseID != c.ExpectedPending.GoalLeaseID ||
			!c.RemainingPending.CreatedAt.Equal(c.ExpectedPending.CreatedAt) ||
			!c.RemainingPending.Capabilities.Equal(c.ExpectedPending.Capabilities) {
			return errors.New("runs: waiting cancellation changed immutable Pending facts")
		}
	} else {
		for _, binding := range c.ExpectedPending.Suspensions {
			if _, canceled := v.canceledProcessIDs[binding.ProcessID]; !canceled {
				return fmt.Errorf("runs: waiting cancellation resumes while suspension %q survives", binding.SuspensionID)
			}
		}
		for _, draft := range c.Resume.Runs {
			dispositionRunIDs = append(dispositionRunIDs, draft.RunID)
		}
	}
	if !slices.Equal(dispositionRunIDs, v.survivingRunIDs) {
		return fmt.Errorf("runs: waiting cancellation disposition Runs %v, want %v", dispositionRunIDs, v.survivingRunIDs)
	}
	if c.RemainingPending == nil {
		return nil
	}
	target := v.continuationByRunID[c.TargetRunID]
	for _, actual := range c.RemainingPending.Continuations {
		expected := v.continuationByRunID[actual.RunID]
		if actual.RunID == target.Lineage.ParentRunID {
			var matched []DrainedTool
			expected.DrainedTools = slices.DeleteFunc(slices.Clone(expected.DrainedTools), func(candidate DrainedTool) bool {
				if candidate.ItemID != c.ParentItem.Expected.ID {
					return false
				}
				matched = append(matched, candidate)
				return true
			})
			if len(matched) != 1 {
				return fmt.Errorf("runs: waiting cancellation parent continuation has %d spawning tools", len(matched))
			}
			settled := matched[0]
			expected.CommittedTools = append(slices.Clone(expected.CommittedTools), CommittedTool{
				ItemID: settled.ItemID, CallID: settled.CallID, Name: settled.Name,
				Arguments: settled.Arguments, Problem: *c.ParentItem.Replacement.Error,
			})
		}
		if !sameContinuationValue(actual, expected) {
			return fmt.Errorf("runs: waiting cancellation changed continuation for Run %q", actual.RunID)
		}
	}
	return nil
}

func (v waitingCancellationValidation) validateOpeningEvents() error {
	c := v.commit
	surviving := make(map[string]struct{}, len(v.survivingRunIDs))
	for _, runID := range v.survivingRunIDs {
		surviving[runID] = struct{}{}
	}
	for index, event := range c.OpeningEvents {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("runs: waiting cancellation opening event[%d]: %w", index, err)
		}
		if event.State != StateUnchanged || event.SessionID != c.SessionID || len(event.Items) == 0 {
			return fmt.Errorf("runs: waiting cancellation opening event[%d] is not item-only", index)
		}
		if _, exists := surviving[event.RunID]; !exists {
			return fmt.Errorf("runs: waiting cancellation opening event[%d] names removed Run %q", index, event.RunID)
		}
	}
	return nil
}

func (v waitingCancellationValidation) validateParentItem() error {
	c := v.commit
	expected, replacement := c.ParentItem.Expected, c.ParentItem.Replacement
	target := v.continuationByRunID[c.TargetRunID]
	if expected.ID == "" || expected.ID != replacement.ID ||
		expected.ID != target.Lineage.SpawnedByItemID || expected.SessionID != c.SessionID ||
		replacement.SessionID != c.SessionID || expected.RunID != replacement.RunID ||
		expected.RunID != target.Lineage.ParentRunID {
		return errors.New("runs: waiting cancellation parent Item identity mismatch")
	}
	withoutProblem := replacement
	withoutProblem.Error = nil
	if expected.Kind != transcript.ToolCall || expected.Tool == nil ||
		expected.Status != transcript.ItemIncomplete || expected.Error != nil ||
		!reflect.DeepEqual(expected, withoutProblem) {
		return errors.New("runs: waiting cancellation parent replacement changes immutable facts")
	}
	if replacement.Status != transcript.ItemIncomplete || replacement.Error == nil ||
		replacement.Error.Kind != transcript.ChildRunCanceledProblem || replacement.Error.Scope != transcript.ToolProblem {
		return errors.New("runs: waiting cancellation parent Item lacks child_run_canceled")
	}
	return nil
}

func validateWaitingRunContinuation(run transcript.Run, continuation Continuation) error {
	switch {
	case run.ID != continuation.RunID:
		return errors.New("identity differs from continuation")
	case run.Lineage() != continuation.Lineage:
		return errors.New("lineage differs from continuation")
	case run.ModelSelection != continuation.ModelSelection:
		return errors.New("model selection differs from continuation")
	case !run.Metrics.Equal(continuation.Metrics):
		return errors.New("metrics differ from continuation")
	case run.Limits != continuation.Limits:
		return errors.New("limits differ from continuation")
	case !run.CreatedAt.Equal(continuation.RunCreatedAt):
		return errors.New("creation time differs from continuation")
	default:
		return nil
	}
}

func sameContinuationValue(left, right Continuation) bool {
	return reflect.DeepEqual(normalizeContinuationValue(left), normalizeContinuationValue(right))
}

func normalizeContinuationValue(value Continuation) Continuation {
	value.RunCreatedAt = canonicalTime(value.RunCreatedAt)
	value.DrainedTools = slices.Clone(value.DrainedTools)
	for index := range value.DrainedTools {
		value.DrainedTools[index].ItemOccurredAt = canonicalTime(value.DrainedTools[index].ItemOccurredAt)
	}
	if len(value.DrainedTools) == 0 {
		value.DrainedTools = nil
	}
	if len(value.CommittedTools) == 0 {
		value.CommittedTools = nil
	}
	if value.Metrics.Usage != nil {
		usage := *value.Metrics.Usage
		if len(usage.ByModel) == 0 {
			usage.ByModel = nil
		}
		value.Metrics.Usage = &usage
	}
	return value
}

func sameInterruptValue(left, right transcript.Interrupt) bool {
	return reflect.DeepEqual(normalizeInterruptValue(left), normalizeInterruptValue(right))
}

func normalizeInterruptValue(value transcript.Interrupt) transcript.Interrupt {
	value.ItemOccurredAt = canonicalTime(value.ItemOccurredAt)
	if value.Question == nil {
		return value
	}
	question := *value.Question
	question.Fields = slices.Clone(question.Fields)
	for index := range question.Fields {
		if len(question.Fields[index].Options) == 0 {
			question.Fields[index].Options = nil
		}
	}
	if len(question.Fields) == 0 {
		question.Fields = nil
	}
	value.Question = &question
	return value
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Unix(0, value.UnixNano()).UTC()
}
