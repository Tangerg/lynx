package runs

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// Validate proves that a boot-recovery write-set is self-contained and
// owner-bound before its transaction begins.
func (r RecoveryCommit) Validate() error {
	lostByID := make(map[string]rundomain.Run, len(r.LostRuns))
	treeMembers := make(map[string][]rundomain.TreeMember)
	actualOrder := make([]string, 0, len(r.LostRuns))
	for index, run := range r.LostRuns {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit lost Run[%d]: %w", index, err)
		}
		outcome, terminal := run.Outcome()
		failure, failed := run.Failure()
		if !terminal || outcome != rundomain.OutcomeLost || !failed || failure.Kind != rundomain.FailureLost {
			return fmt.Errorf("runs: recovery commit Run %q is not a run-lost terminal", run.ID())
		}
		if _, duplicate := lostByID[run.ID()]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats lost Run %q", run.ID())
		}
		lostByID[run.ID()] = run
		rootID := run.Lineage().TreeRootID(run.ID())
		treeMembers[rootID] = append(treeMembers[rootID], rundomain.TreeMember{
			RunID:   run.ID(),
			Lineage: run.Lineage(),
		})
		actualOrder = append(actualOrder, run.ID())
	}
	rootIDs := make([]string, 0, len(treeMembers))
	for rootID := range treeMembers {
		rootIDs = append(rootIDs, rootID)
	}
	slices.Sort(rootIDs)
	expectedOrder := make([]string, 0, len(r.LostRuns))
	for _, rootID := range rootIDs {
		members := treeMembers[rootID]
		tree, err := rundomain.NewTree(rootID, members)
		if err != nil {
			return fmt.Errorf("runs: recovery commit tree %q: %w", rootID, err)
		}
		expectedOrder = append(expectedOrder, tree.Postorder()...)
	}
	if !slices.Equal(actualOrder, expectedOrder) {
		return errors.New("runs: recovery commit lost Runs are not in canonical tree/postorder")
	}
	if err := validateRecoveryConversationTransitions(
		r.ConversationTransitions,
		rootIDs,
		treeMembers,
		lostByID,
	); err != nil {
		return err
	}
	if err := validateRecoveryModelInvocations(r.ModelInvocations, lostByID); err != nil {
		return err
	}
	if err := validateRecoveryToolInvocations(r.ToolInvocations, lostByID); err != nil {
		return err
	}

	replacedItems := make(map[string]struct{}, len(r.ItemReplacements))
	for index, replacement := range r.ItemReplacements {
		owner, found := lostByID[replacement.Expected.RunID()]
		if !found || replacement.Expected.SessionID() != owner.SessionID() {
			return fmt.Errorf(
				"runs: recovery commit Item %q is not owned by a lost Run",
				replacement.Expected.ID(),
			)
		}
		if err := validateRecoveryItemReplacement(replacement, owner.FinishedAt()); err != nil {
			return fmt.Errorf("runs: recovery commit Item replacement[%d]: %w", index, err)
		}
		if _, duplicate := replacedItems[replacement.Expected.ID()]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Item replacement %q", replacement.Expected.ID())
		}
		replacedItems[replacement.Expected.ID()] = struct{}{}
	}
	if err := validateRecoveryGoalRuns(r.GoalRuns, lostByID); err != nil {
		return err
	}
	if err := validateRecoveryInterruptDeletions(r.DeleteInterrupts, lostByID); err != nil {
		return err
	}
	if err := validateCanonicalIdentities("preserved checkpoint root", r.PreservedCheckpointRootIDs); err != nil {
		return err
	}
	if err := validateCanonicalIdentities("recovered Session", r.RecoveredSessionIDs); err != nil {
		return err
	}
	if err := validateCanonicalIdentities("checkpoint deletion Session", r.DeleteCheckpointSessionIDs); err != nil {
		return err
	}
	return nil
}

func validateRecoveryModelInvocations(
	invocations []ModelInvocationRecovery,
	lostByID map[string]rundomain.Run,
) error {
	seen := make(map[string]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := validateRecoveryInvocation(
			invocation.SessionID,
			invocation.RunID,
			invocation.SegmentID,
			invocation.CallID,
			invocation.StartedAt,
			invocation.FinishedAt,
			lostByID,
		); err != nil {
			return fmt.Errorf("runs: recovery commit model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seen[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats model invocation %q", invocation.CallID)
		}
		seen[invocation.CallID] = struct{}{}
		if index > 0 && compareModelInvocationRecoveries(invocations[index-1], invocation) >= 0 {
			return errors.New("runs: recovery commit model invocations are not in canonical order")
		}
	}
	return nil
}

func validateRecoveryToolInvocations(
	invocations []ToolInvocationRecovery,
	lostByID map[string]rundomain.Run,
) error {
	seen := make(map[string]struct{}, len(invocations))
	seenItems := make(map[string]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := validateRecoveryInvocation(
			invocation.SessionID,
			invocation.RunID,
			invocation.SegmentID,
			invocation.CallID,
			invocation.StartedAt,
			invocation.FinishedAt,
			lostByID,
		); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
		}
		if err := validateRecoveryIdentity("Item", invocation.ItemID); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
		}
		key := invocation.CallID + "\x00" + invocation.SegmentID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: recovery commit repeats Tool invocation %q in Segment %q",
				invocation.CallID,
				invocation.SegmentID,
			)
		}
		seen[key] = struct{}{}
		itemKey := invocation.ItemID + "\x00" + invocation.SegmentID
		if _, duplicate := seenItems[itemKey]; duplicate {
			return fmt.Errorf(
				"runs: recovery commit repeats Tool invocation Item %q in Segment %q",
				invocation.ItemID,
				invocation.SegmentID,
			)
		}
		seenItems[itemKey] = struct{}{}
		if index > 0 && compareToolInvocationRecoveries(invocations[index-1], invocation) >= 0 {
			return errors.New("runs: recovery commit Tool invocations are not in canonical order")
		}
	}
	return nil
}

func validateRecoveryInvocation(
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	lostByID map[string]rundomain.Run,
) error {
	for _, identity := range []struct {
		name  string
		value string
	}{
		{name: "Session", value: sessionID},
		{name: "Run", value: runID},
		{name: "Segment", value: segmentID},
		{name: "call", value: callID},
	} {
		if err := validateRecoveryIdentity(identity.name, identity.value); err != nil {
			return err
		}
	}
	if startedAt.IsZero() || finishedAt.IsZero() {
		return errors.New("invocation start and finish times are required")
	}
	if finishedAt.Before(startedAt) {
		return errors.New("invocation finish time precedes start time")
	}
	if lost, found := lostByID[runID]; found {
		if lost.SessionID() != sessionID || !lost.FinishedAt().Equal(finishedAt) {
			return fmt.Errorf("invocation differs from its recovered lost Run %q", runID)
		}
	}
	return nil
}

func validateRecoveryIdentity(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s ID is required without surrounding whitespace", name)
	}
	return nil
}

func validateRecoveryConversationTransitions(
	transitions []RecoveryConversationTransition,
	rootIDs []string,
	treeMembers map[string][]rundomain.TreeMember,
	lostByID map[string]rundomain.Run,
) error {
	if len(transitions) != len(rootIDs) {
		return fmt.Errorf(
			"runs: recovery commit has %d conversation transitions, want %d lost roots",
			len(transitions),
			len(rootIDs),
		)
	}
	for index, rootID := range rootIDs {
		transition := transitions[index]
		root := lostByID[rootID]
		if transition.RootRunID != rootID ||
			transition.SessionID != root.SessionID() ||
			strings.TrimSpace(transition.SessionID) == "" ||
			transition.SessionID != strings.TrimSpace(transition.SessionID) ||
			transition.ExpectedCount < 0 {
			return fmt.Errorf(
				"runs: recovery commit conversation transition[%d] differs from lost root Run %q",
				index,
				rootID,
			)
		}
		if len(transition.Messages) > 1 {
			return fmt.Errorf(
				"runs: recovery commit conversation transition for root Run %q has more than one closure message",
				rootID,
			)
		}
		seenToolCalls := make(map[string]struct{})
		for messageIndex, message := range transition.Messages {
			if err := message.Validate(); err != nil {
				return fmt.Errorf(
					"runs: recovery commit conversation transition for root Run %q message[%d]: %w",
					rootID,
					messageIndex,
					err,
				)
			}
			if message.Role != corechat.RoleTool {
				return fmt.Errorf(
					"runs: recovery commit conversation transition for root Run %q is not a Tool message",
					rootID,
				)
			}
			for _, part := range message.Parts {
				result := part.ToolResult
				if result == nil || !result.IsError || result.Result != recoveryLostToolResult {
					return fmt.Errorf(
						"runs: recovery commit conversation transition for root Run %q has an invalid Tool result",
						rootID,
					)
				}
				if _, duplicate := seenToolCalls[result.ID]; duplicate {
					return fmt.Errorf(
						"runs: recovery commit conversation transition for root Run %q repeats ToolCall %q",
						rootID,
						result.ID,
					)
				}
				seenToolCalls[result.ID] = struct{}{}
			}
		}
		messageMark := transition.ExpectedCount + len(transition.Messages)
		for _, member := range treeMembers[rootID] {
			if lostByID[member.RunID].MessageMark() != messageMark {
				return fmt.Errorf(
					"runs: recovery commit lost Run %q message mark differs from its conversation transition",
					member.RunID,
				)
			}
		}
	}
	return nil
}

func validateRecoveryItemReplacement(replacement ItemReplacement, finishedAt time.Time) error {
	expected := replacement.Expected
	actual := replacement.Replacement
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("expected Item: %w", err)
	}
	if err := actual.Validate(); err != nil {
		return fmt.Errorf("replacement Item: %w", err)
	}
	if expected.ID() == "" || expected.SessionID() == "" || expected.RunID() == "" {
		return errors.New("expected Item identity is incomplete")
	}
	if expected.Status() != transcript.ItemRunning || actual.Status() != transcript.ItemIncomplete {
		return errors.New("replacement must move a Running Item to Incomplete")
	}
	failure := tool.Failure{
		Kind:   tool.FailureExecution,
		Detail: "tool call interrupted because the run was lost on restart",
	}
	want, err := expected.AbandonToolCall(&failure, finishedAt)
	if err != nil {
		return fmt.Errorf("expected recovery transition: %w", err)
	}
	if !reflect.DeepEqual(actual.Snapshot(), want.Snapshot()) {
		return fmt.Errorf("replacement rewrites facts other than recovery status for Item %q", expected.ID())
	}
	return nil
}

func validateRecoveryGoalRuns(records []goal.RunRecord, lostByID map[string]rundomain.Run) error {
	expected := make(map[string]rundomain.Run)
	for _, run := range lostByID {
		if run.Lineage().IsRoot() && run.GoalIncarnationID() != "" {
			expected[run.ID()] = run
		}
	}
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit Goal Run[%d]: %w", index, err)
		}
		if _, duplicate := seen[record.RunID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Goal Run for Run %q", record.RunID)
		}
		seen[record.RunID] = struct{}{}
		run, found := expected[record.RunID]
		outcome, terminal := run.Outcome()
		if !found || !terminal {
			return fmt.Errorf("runs: recovery commit Goal Run names unowned Run %q", record.RunID)
		}
		cost := 0.0
		if usage, reported := run.Metrics().Usage(); reported && usage.Total.CostUSD != nil {
			cost = *usage.Total.CostUSD
		}
		if record.SessionID != run.SessionID() || record.IncarnationID != run.GoalIncarnationID() ||
			record.Outcome != outcome || record.CostUSD != cost ||
			record.Steps != run.Metrics().Steps() || !record.CompletedAt.Equal(run.FinishedAt()) {
			return fmt.Errorf("runs: recovery commit Goal Run differs from lost Run %q", run.ID())
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d Goal Runs, want %d", len(seen), len(expected))
	}
	return nil
}

func validateRecoveryInterruptDeletions(
	values []InterruptOwner,
	lostByID map[string]rundomain.Run,
) error {
	expected := make(map[string]rundomain.Run)
	for _, lost := range lostByID {
		if lost.Lineage().IsRoot() {
			expected[lost.ID()] = lost
		}
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value.SessionID) == "" || value.SessionID != strings.TrimSpace(value.SessionID) ||
			strings.TrimSpace(value.RootRunID) == "" || value.RootRunID != strings.TrimSpace(value.RootRunID) {
			return fmt.Errorf("runs: recovery commit interrupt deletion[%d] has invalid identity", index)
		}
		key := value.SessionID + "\x00" + value.RootRunID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats interrupt deletion %q/%q", value.SessionID, value.RootRunID)
		}
		seen[key] = struct{}{}
		owner, found := expected[value.RootRunID]
		if !found || owner.SessionID() != value.SessionID {
			return fmt.Errorf(
				"runs: recovery commit interrupt deletion %q/%q is not owned by a lost root Run",
				value.SessionID,
				value.RootRunID,
			)
		}
		if index > 0 {
			previous := values[index-1]
			if previous.SessionID > value.SessionID ||
				(previous.SessionID == value.SessionID && previous.RootRunID >= value.RootRunID) {
				return errors.New("runs: recovery commit interrupt deletions are not in canonical order")
			}
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d interrupt deletions, want %d lost roots", len(seen), len(expected))
	}
	return nil
}

func validateCanonicalIdentities(name string, values []string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("runs: recovery commit %s[%d] is invalid", name, index)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("runs: recovery commit %ss are not unique canonical order", name)
		}
	}
	return nil
}
