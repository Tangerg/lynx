package runs

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
	rundomain "github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

type waitingCancellationValidation struct {
	commit              WaitingSubtreeCancellationCommit
	continuationByRunID map[string]Continuation
	canceledRunIDs      []string
	canceledMemberIDs   map[string]struct{}
	survivingRunIDs     []string
	terminalRunIDs      map[string]struct{}
	finishedAtByRunID   map[string]time.Time
}

// Validate proves that the write-set is exactly the canonical transformation
// of ExpectedPending after TargetRunID's subtree is removed. This is application
// policy: persistence only claims the frozen Pending snapshot and writes these
// already-validated facts atomically.
func (w WaitingSubtreeCancellationCommit) Validate() error {
	validation, err := newWaitingCancellationValidation(w)
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
	if err := validation.validateConversationMessages(); err != nil {
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
	if err := validateWaitingCancellationBoundary(c); err != nil {
		return waitingCancellationValidation{}, err
	}
	if err := validateWaitingCancellationDispositionEnvelope(c); err != nil {
		return waitingCancellationValidation{}, err
	}
	topology, err := buildWaitingCancellationTopology(c)
	if err != nil {
		return waitingCancellationValidation{}, err
	}
	return waitingCancellationValidation{
		commit:              c,
		continuationByRunID: topology.continuationByRunID,
		canceledRunIDs:      topology.canceledRunIDs,
		canceledMemberIDs:   topology.canceledMemberIDs,
		survivingRunIDs:     topology.survivingRunIDs,
		terminalRunIDs:      make(map[string]struct{}, len(c.TerminalRuns)),
		finishedAtByRunID:   make(map[string]time.Time, len(c.TerminalRuns)),
	}, nil
}

func validateWaitingCancellationBoundary(c WaitingSubtreeCancellationCommit) error {
	if strings.TrimSpace(c.CommitID) == "" || c.CommitID != strings.TrimSpace(c.CommitID) {
		return errors.New("runs: waiting cancellation commit identity is required without surrounding whitespace")
	}
	if strings.TrimSpace(c.RootRunID) == "" || strings.TrimSpace(c.TargetRunID) == "" ||
		strings.TrimSpace(c.SessionID) == "" {
		return errors.New("runs: waiting cancellation identity is incomplete")
	}
	if c.RootRun.ID() != c.RootRunID || c.RootRun.SessionID() != c.SessionID ||
		!c.RootRun.Lineage().IsRoot() || c.RootRun.State() != rundomain.Waiting {
		return errors.New("runs: waiting cancellation root snapshot is invalid")
	}
	if err := c.ExpectedPending.Validate(); err != nil {
		return fmt.Errorf("runs: waiting cancellation expected Pending: %w", err)
	}
	if c.ExpectedPending.RootRunID != c.RootRunID || c.ExpectedPending.SessionID != c.SessionID {
		return errors.New("runs: waiting cancellation expected Pending scope mismatch")
	}
	rootContinuation, found := c.ExpectedPending.RootContinuation()
	if !found {
		return errors.New("runs: waiting cancellation expected Pending has no root continuation")
	}
	if c.RootRun.GoalIncarnationID() != c.ExpectedPending.GoalIncarnationID {
		return errors.New("runs: waiting cancellation root Run goal incarnation differs from Pending")
	}
	if !c.RootRun.Capabilities().Equal(c.ExpectedPending.Capabilities) {
		return errors.New("runs: waiting cancellation root Run capabilities differ from Pending")
	}
	if err := validateWaitingRunContinuation(c.RootRun, rootContinuation); err != nil {
		return fmt.Errorf("runs: waiting cancellation root Run: %w", err)
	}
	if err := c.Checkpoint.ValidateOwnership(rootContinuation.MemberID, c.SessionID); err != nil {
		return fmt.Errorf("runs: waiting cancellation checkpoint ownership: %w", err)
	}
	if c.Checkpoint.Scope.GoalIncarnationID != c.ExpectedPending.GoalIncarnationID ||
		c.Checkpoint.ModelSelection != rootContinuation.ModelSelection ||
		c.Checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf(
			"runs: waiting cancellation checkpoint differs from root continuation: %w",
			ErrInvalidExecutorCheckpoint,
		)
	}
	return nil
}

func validateWaitingCancellationDispositionEnvelope(c WaitingSubtreeCancellationCommit) error {
	if (c.RemainingPending == nil) == (c.Resume == nil) {
		return errors.New("runs: waiting cancellation requires exactly one surviving disposition")
	}
	if c.RemainingPending == nil {
		if err := c.Resume.Validate(); err != nil {
			return fmt.Errorf("runs: waiting cancellation Resume: %w", err)
		}
		if c.Resume.RootRunID != c.RootRunID || c.Resume.SessionID != c.SessionID {
			return errors.New("runs: waiting cancellation Resume scope mismatch")
		}
		return nil
	}
	if err := c.RemainingPending.Validate(); err != nil {
		return fmt.Errorf("runs: waiting cancellation reduced Pending: %w", err)
	}
	if c.RemainingPending.RootRunID != c.RootRunID || c.RemainingPending.SessionID != c.SessionID {
		return errors.New("runs: waiting cancellation reduced Pending scope mismatch")
	}
	if len(c.OpeningEvents) != 0 {
		return errors.New("runs: waiting cancellation still parked carries opening events")
	}
	return nil
}

type waitingCancellationTopology struct {
	continuationByRunID map[string]Continuation
	canceledRunIDs      []string
	canceledMemberIDs   map[string]struct{}
	survivingRunIDs     []string
}

func buildWaitingCancellationTopology(
	c WaitingSubtreeCancellationCommit,
) (waitingCancellationTopology, error) {
	members := make([]rundomain.TreeMember, 0, len(c.ExpectedPending.Continuations))
	continuationByRunID := make(map[string]Continuation, len(c.ExpectedPending.Continuations))
	for _, continuation := range c.ExpectedPending.Continuations {
		members = append(members, rundomain.TreeMember{RunID: continuation.RunID, Lineage: continuation.Lineage})
		continuationByRunID[continuation.RunID] = continuation
	}
	tree, err := rundomain.NewTree(c.RootRunID, members)
	if err != nil {
		return waitingCancellationTopology{}, fmt.Errorf("runs: waiting cancellation tree: %w", err)
	}
	canceledRunIDs, found := tree.SubtreePostorder(c.TargetRunID)
	if !found || c.TargetRunID == c.RootRunID {
		return waitingCancellationTopology{}, errors.New("runs: waiting cancellation target is not a child in the pending tree")
	}
	canceledRunSet := make(map[string]struct{}, len(canceledRunIDs))
	canceledMemberIDs := make(map[string]struct{}, len(canceledRunIDs))
	for _, runID := range canceledRunIDs {
		canceledRunSet[runID] = struct{}{}
		canceledMemberIDs[continuationByRunID[runID].MemberID] = struct{}{}
	}
	var survivingRunIDs []string
	for _, runID := range tree.Postorder() {
		if _, canceled := canceledRunSet[runID]; !canceled {
			survivingRunIDs = append(survivingRunIDs, runID)
		}
	}
	return waitingCancellationTopology{
		continuationByRunID: continuationByRunID,
		canceledRunIDs:      canceledRunIDs,
		canceledMemberIDs:   canceledMemberIDs,
		survivingRunIDs:     survivingRunIDs,
	}, nil
}

func (w *waitingCancellationValidation) validateTerminalRuns() error {
	c := w.commit
	if len(c.TerminalRuns) != len(w.canceledRunIDs) {
		return fmt.Errorf(
			"runs: waiting cancellation has %d terminal Runs, target subtree requires %d",
			len(c.TerminalRuns),
			len(w.canceledRunIDs),
		)
	}
	for index, run := range c.TerminalRuns {
		expectedRunID := w.canceledRunIDs[index]
		continuation := w.continuationByRunID[expectedRunID]
		switch {
		case run.ID() != expectedRunID:
			return fmt.Errorf("runs: waiting cancellation Run[%d] is %q, want %q", index, run.ID(), expectedRunID)
		case run.SessionID() != c.SessionID:
			return fmt.Errorf("runs: waiting cancellation Run[%d] Session mismatch", index)
		case run.Lineage() != continuation.Lineage:
			return fmt.Errorf("runs: waiting cancellation Run[%d] lineage mismatch", index)
		case run.ModelSelection() != continuation.ModelSelection:
			return fmt.Errorf("runs: waiting cancellation Run[%d] model mismatch", index)
		case !run.Metrics().Equal(continuation.Metrics):
			return fmt.Errorf("runs: waiting cancellation Run[%d] metrics mismatch", index)
		case run.Limits() != continuation.Limits:
			return fmt.Errorf("runs: waiting cancellation Run[%d] limits mismatch", index)
		case !run.CreatedAt().Equal(continuation.RunCreatedAt):
			return fmt.Errorf("runs: waiting cancellation Run[%d] creation time mismatch", index)
		case !run.Capabilities().Equal(c.ExpectedPending.Capabilities):
			return fmt.Errorf("runs: waiting cancellation Run[%d] capabilities mismatch", index)
		case run.GoalIncarnationID() != "":
			return fmt.Errorf("runs: waiting cancellation child Run[%d] carries a root Goal incarnation", index)
		case run.State() != rundomain.Canceled:
			return fmt.Errorf("runs: waiting cancellation Run[%d] is not canceled", index)
		}
		outcome, terminal := run.Outcome()
		if !terminal || outcome != rundomain.OutcomeCanceled {
			return fmt.Errorf("runs: waiting cancellation Run[%d] has no canceled outcome", index)
		}
		if _, duplicate := w.terminalRunIDs[run.ID()]; duplicate {
			return fmt.Errorf("runs: waiting cancellation repeats Run %q", run.ID())
		}
		w.terminalRunIDs[run.ID()] = struct{}{}
		w.finishedAtByRunID[run.ID()] = run.FinishedAt()
	}
	return nil
}

func (w waitingCancellationValidation) validateTerminalItems() error {
	c := w.commit
	type expectedTool struct {
		runID     string
		name      string
		arguments string
	}
	expectedByItemID := make(map[string]expectedTool)
	for _, request := range c.ExpectedPending.Interrupts {
		if _, terminal := w.terminalRunIDs[request.RunID]; terminal && request.Kind == interrupt.Approval {
			expectedByItemID[request.ItemID] = expectedTool{
				runID: request.RunID, name: request.Approval.Tool.Name,
				arguments: request.Approval.Tool.Arguments.Canonical(),
			}
		}
	}
	for _, continuation := range c.ExpectedPending.Continuations {
		if _, terminal := w.terminalRunIDs[continuation.RunID]; !terminal {
			continue
		}
		for _, drained := range continuation.DrainedTools {
			if _, duplicate := expectedByItemID[drained.ItemID]; duplicate {
				return fmt.Errorf("runs: waiting cancellation Tool Item %q has multiple terminal roles", drained.ItemID)
			}
			expectedByItemID[drained.ItemID] = expectedTool{
				runID: continuation.RunID, name: drained.Name, arguments: drained.Arguments,
			}
		}
	}
	if len(c.TerminalItems) != len(expectedByItemID) {
		return fmt.Errorf("runs: waiting cancellation has %d terminal Items, want %d", len(c.TerminalItems), len(expectedByItemID))
	}
	seen := make(map[string]struct{}, len(c.TerminalItems))
	for index, replacement := range c.TerminalItems {
		expectedTool, expected := expectedByItemID[replacement.Expected.ID()]
		if !expected || replacement.Expected.SessionID() != c.SessionID ||
			replacement.Expected.RunID() != expectedTool.runID || replacement.Expected.Status() != transcript.ItemRunning {
			return fmt.Errorf("runs: waiting cancellation terminal Item[%d] is outside canceled Tools", index)
		}
		if _, duplicate := seen[replacement.Expected.ID()]; duplicate {
			return fmt.Errorf("runs: waiting cancellation repeats terminal Item %q", replacement.Expected.ID())
		}
		seen[replacement.Expected.ID()] = struct{}{}
		if err := validateTerminalItemReplacement(
			expectedTool.name,
			expectedTool.arguments,
			replacement,
			w.finishedAtByRunID[expectedTool.runID],
		); err != nil {
			return fmt.Errorf("runs: waiting cancellation terminal Item %q: %w", replacement.Expected.ID(), err)
		}
	}
	return nil
}

func validateTerminalItemReplacement(
	toolName string,
	arguments string,
	replacement ItemReplacement,
	finishedAt time.Time,
) error {
	invocation, present := replacement.Expected.ToolInvocation()
	if replacement.Expected.Kind() != transcript.ToolCall || !present ||
		invocation.Name != toolName || invocation.Arguments.Canonical() != arguments {
		return errors.New("tool Item differs from its pending identity")
	}
	failure, failed := replacement.Replacement.Failure()
	if !failed || failure.Kind != tool.FailureExecution {
		return errors.New("tool replacement has an invalid failure")
	}
	expected, err := replacement.Expected.AbandonToolCall(&failure, finishedAt)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected.Snapshot(), replacement.Replacement.Snapshot()) {
		return errors.New("replacement changes facts beyond terminal status")
	}
	return nil
}

func (w waitingCancellationValidation) validateDisposition() error {
	dispositionRunIDs, err := w.validateDispositionAndCollectRunIDs()
	if err != nil {
		return err
	}
	if !slices.Equal(dispositionRunIDs, w.survivingRunIDs) {
		return fmt.Errorf("runs: waiting cancellation disposition Runs %v, want %v", dispositionRunIDs, w.survivingRunIDs)
	}
	if w.commit.RemainingPending == nil {
		return nil
	}
	return w.validateSurvivingContinuations()
}

func (w waitingCancellationValidation) validateDispositionAndCollectRunIDs() ([]string, error) {
	if w.commit.RemainingPending != nil {
		return w.validateReducedPendingAndCollectRunIDs()
	}
	for _, binding := range w.commit.ExpectedPending.Bindings {
		if _, canceled := w.canceledMemberIDs[binding.MemberID]; !canceled {
			return nil, fmt.Errorf(
				"runs: waiting cancellation resumes while input request %q survives",
				binding.RequestID,
			)
		}
	}
	runIDs := make([]string, 0, len(w.commit.Resume.Runs))
	for _, draft := range w.commit.Resume.Runs {
		runIDs = append(runIDs, draft.RunID)
	}
	return runIDs, nil
}

func (w waitingCancellationValidation) validateReducedPendingAndCollectRunIDs() ([]string, error) {
	c := w.commit
	var survivingRequestIndexes []int
	for index, binding := range c.ExpectedPending.Bindings {
		if _, canceled := w.canceledMemberIDs[binding.MemberID]; !canceled {
			survivingRequestIndexes = append(survivingRequestIndexes, index)
		}
	}
	if len(c.RemainingPending.Bindings) != len(survivingRequestIndexes) {
		return nil, errors.New("runs: waiting cancellation reduced Pending has the wrong input-request set")
	}
	for index, expectedIndex := range survivingRequestIndexes {
		if c.RemainingPending.Bindings[index] != c.ExpectedPending.Bindings[expectedIndex] ||
			!sameInterruptValue(c.RemainingPending.Interrupts[index], c.ExpectedPending.Interrupts[expectedIndex]) {
			return nil, fmt.Errorf("runs: waiting cancellation changed surviving input request[%d]", index)
		}
	}
	if c.RemainingPending.ExecutorID != c.ExpectedPending.ExecutorID ||
		c.RemainingPending.GoalIncarnationID != c.ExpectedPending.GoalIncarnationID ||
		!c.RemainingPending.CreatedAt.Equal(c.ExpectedPending.CreatedAt) ||
		!c.RemainingPending.Capabilities.Equal(c.ExpectedPending.Capabilities) {
		return nil, errors.New("runs: waiting cancellation changed immutable Pending facts")
	}
	runIDs := make([]string, 0, len(c.RemainingPending.Continuations))
	for _, continuation := range c.RemainingPending.Continuations {
		runIDs = append(runIDs, continuation.RunID)
	}
	return runIDs, nil
}

func (w waitingCancellationValidation) validateSurvivingContinuations() error {
	c := w.commit
	target := w.continuationByRunID[c.TargetRunID]
	for _, actual := range c.RemainingPending.Continuations {
		expected := w.continuationByRunID[actual.RunID]
		if actual.RunID == target.Lineage.ParentRunID {
			failure, failed := c.ParentItem.Replacement.Failure()
			if !failed {
				return errors.New("runs: waiting cancellation parent replacement has no failure")
			}
			var matched []DrainedTool
			expected.DrainedTools = slices.DeleteFunc(slices.Clone(expected.DrainedTools), func(candidate DrainedTool) bool {
				if candidate.ItemID != c.ParentItem.Expected.ID() {
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
				ItemID: settled.ItemID, CallID: settled.CallID, SourceCallID: settled.SourceCallID,
				Name: settled.Name, Arguments: settled.Arguments, Failure: failure,
			})
		}
		if !sameContinuationValue(actual, expected) {
			return fmt.Errorf("runs: waiting cancellation changed continuation for Run %q", actual.RunID)
		}
	}
	return nil
}

func (w waitingCancellationValidation) validateOpeningEvents() error {
	c := w.commit
	surviving := make(map[string]struct{}, len(w.survivingRunIDs))
	for _, runID := range w.survivingRunIDs {
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

func (w waitingCancellationValidation) validateParentItem() error {
	c := w.commit
	expected, replacement := c.ParentItem.Expected, c.ParentItem.Replacement
	target := w.continuationByRunID[c.TargetRunID]
	if expected.ID() == "" || expected.ID() != replacement.ID() ||
		expected.ID() != target.Lineage.SpawnedByItemID || expected.SessionID() != c.SessionID ||
		replacement.SessionID() != c.SessionID || expected.RunID() != replacement.RunID() ||
		expected.RunID() != target.Lineage.ParentRunID {
		return errors.New("runs: waiting cancellation parent Item identity mismatch")
	}
	if expected.Kind() != transcript.ToolCall || expected.Status() != transcript.ItemRunning {
		return errors.New("runs: waiting cancellation parent replacement changes immutable facts")
	}
	if _, present := expected.ToolInvocation(); !present {
		return errors.New("runs: waiting cancellation parent Item has no invocation")
	}
	if _, failed := expected.Failure(); failed {
		return errors.New("runs: waiting cancellation parent Item already has a failure")
	}
	failure, failed := replacement.Failure()
	if replacement.Status() != transcript.ItemIncomplete || !failed ||
		failure.Kind != tool.FailureChildRunCanceled {
		return errors.New("runs: waiting cancellation parent Item lacks child_run_canceled")
	}
	want, err := expected.AbandonToolCall(&failure, replacement.FinishedAt())
	if err != nil || !reflect.DeepEqual(want.Snapshot(), replacement.Snapshot()) {
		return errors.New("runs: waiting cancellation parent replacement changes immutable facts")
	}
	return nil
}

func (w waitingCancellationValidation) validateConversationMessages() error {
	c := w.commit
	if c.ParentItem.Expected.RunID() != c.RootRunID {
		if len(c.ConversationMessages) != 0 {
			return errors.New("runs: non-root child cancellation carries root conversation messages")
		}
		return nil
	}
	parentContinuation := w.continuationByRunID[c.RootRunID]
	var committed CommittedTool
	found := false
	for _, drained := range parentContinuation.DrainedTools {
		if drained.ItemID != c.ParentItem.Expected.ID() {
			continue
		}
		committed = CommittedTool{
			ItemID: drained.ItemID, CallID: drained.CallID, SourceCallID: drained.SourceCallID,
			Name: drained.Name, Arguments: drained.Arguments,
		}
		found = true
		break
	}
	failure, failed := c.ParentItem.Replacement.Failure()
	if !found || committed.SourceCallID == "" || !failed {
		return errors.New("runs: root child cancellation cannot correlate its model-context Tool result")
	}
	want := []corechat.Message{childCancellationToolMessage(committed, failure)}
	if !reflect.DeepEqual(want, c.ConversationMessages) {
		return errors.New("runs: waiting cancellation conversation result differs from its parent Tool")
	}
	return nil
}

func validateWaitingRunContinuation(run rundomain.Run, continuation Continuation) error {
	switch {
	case run.ID() != continuation.RunID:
		return errors.New("identity differs from continuation")
	case run.Lineage() != continuation.Lineage:
		return errors.New("lineage differs from continuation")
	case run.ModelSelection() != continuation.ModelSelection:
		return errors.New("model selection differs from continuation")
	case !run.Metrics().Equal(continuation.Metrics):
		return errors.New("metrics differ from continuation")
	case run.Limits() != continuation.Limits:
		return errors.New("limits differ from continuation")
	case !run.CreatedAt().Equal(continuation.RunCreatedAt):
		return errors.New("creation time differs from continuation")
	default:
		return nil
	}
}

func sameContinuationValue(left, right Continuation) bool {
	if !left.Metrics.Equal(right.Metrics) {
		return false
	}
	left.Metrics = rundomain.Metrics{}
	right.Metrics = rundomain.Metrics{}
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
