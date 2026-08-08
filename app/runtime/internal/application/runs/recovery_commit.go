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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Validate proves that a boot-recovery write-set is self-contained and
// owner-bound before its transaction begins.
func (commit RecoveryCommit) Validate() error {
	lostByID := make(map[string]transcript.Run, len(commit.LostRuns))
	treeMembers := make(map[string][]rundomain.RunTreeMember)
	actualOrder := make([]string, 0, len(commit.LostRuns))
	for index, run := range commit.LostRuns {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit lost Run[%d]: %w", index, err)
		}
		if run.Outcome == nil || *run.Outcome != rundomain.OutcomeLost ||
			run.Error == nil || run.Error.Kind != transcript.RunLostProblem {
			return fmt.Errorf("runs: recovery commit Run %q is not a run-lost terminal", run.ID)
		}
		if _, duplicate := lostByID[run.ID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats lost Run %q", run.ID)
		}
		lostByID[run.ID] = run
		rootID := run.Lineage().TreeRootID(run.ID)
		treeMembers[rootID] = append(treeMembers[rootID], rundomain.RunTreeMember{
			RunID:   run.ID,
			Lineage: run.Lineage(),
		})
		actualOrder = append(actualOrder, run.ID)
	}
	rootIDs := make([]string, 0, len(treeMembers))
	for rootID := range treeMembers {
		rootIDs = append(rootIDs, rootID)
	}
	slices.Sort(rootIDs)
	expectedOrder := make([]string, 0, len(commit.LostRuns))
	for _, rootID := range rootIDs {
		members := treeMembers[rootID]
		tree, err := rundomain.NewRunTree(rootID, members)
		if err != nil {
			return fmt.Errorf("runs: recovery commit tree %q: %w", rootID, err)
		}
		expectedOrder = append(expectedOrder, tree.Postorder()...)
	}
	if !slices.Equal(actualOrder, expectedOrder) {
		return errors.New("runs: recovery commit lost Runs are not in canonical tree/postorder")
	}

	replacedItems := make(map[string]struct{}, len(commit.ItemReplacements))
	for index, replacement := range commit.ItemReplacements {
		owner, found := lostByID[replacement.Expected.RunID]
		if !found || replacement.Expected.SessionID != owner.SessionID {
			return fmt.Errorf(
				"runs: recovery commit Item %q is not owned by a lost Run",
				replacement.Expected.ID,
			)
		}
		if err := validateRecoveryItemReplacement(replacement, owner.FinishedAt); err != nil {
			return fmt.Errorf("runs: recovery commit Item replacement[%d]: %w", index, err)
		}
		if _, duplicate := replacedItems[replacement.Expected.ID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Item replacement %q", replacement.Expected.ID)
		}
		replacedItems[replacement.Expected.ID] = struct{}{}
	}
	if err := validateRecoveryGoalRuns(commit.GoalRuns, lostByID); err != nil {
		return err
	}
	if err := validatePendingDeletions(commit.DeletePending, lostByID); err != nil {
		return err
	}
	if err := validateCanonicalIdentities("preserved checkpoint root", commit.PreservedCheckpointRootIDs); err != nil {
		return err
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
	if expected.ID == "" || expected.SessionID == "" || expected.RunID == "" {
		return errors.New("expected Item identity is incomplete")
	}
	if expected.Status != transcript.ItemRunning || actual.Status != transcript.ItemIncomplete {
		return errors.New("replacement must move a Running Item to Incomplete")
	}
	want := expected
	want.Status = transcript.ItemIncomplete
	if want.Kind == transcript.ToolCall {
		want.FinishedAt = finishedAt
		want.Error = &transcript.Problem{
			Kind:   transcript.ToolFailedProblem,
			Scope:  transcript.ToolProblem,
			Detail: "tool call interrupted because the run was lost on restart",
		}
	}
	if !reflect.DeepEqual(actual, want) {
		return fmt.Errorf("replacement rewrites facts other than recovery status for Item %q", expected.ID)
	}
	return nil
}

func validateRecoveryGoalRuns(records []goal.RunRecord, lostByID map[string]transcript.Run) error {
	expected := make(map[string]transcript.Run)
	for _, run := range lostByID {
		if run.Lineage().IsRoot() && run.GoalLeaseID != "" {
			expected[run.ID] = run
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
		if !found || run.Outcome == nil {
			return fmt.Errorf("runs: recovery commit Goal Run names unowned Run %q", record.RunID)
		}
		cost := 0.0
		if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
			cost = *run.Metrics.Usage.CostUSD
		}
		if record.SessionID != run.SessionID || record.LeaseID != run.GoalLeaseID ||
			record.Outcome != *run.Outcome || record.CostUSD != cost ||
			record.Steps != run.Metrics.Steps || !record.CompletedAt.Equal(run.FinishedAt) {
			return fmt.Errorf("runs: recovery commit Goal Run differs from lost Run %q", run.ID)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d Goal Runs, want %d", len(seen), len(expected))
	}
	return nil
}

func validatePendingDeletions(
	values []PendingDeletion,
	lostByID map[string]transcript.Run,
) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value.SessionID) == "" || value.SessionID != strings.TrimSpace(value.SessionID) ||
			strings.TrimSpace(value.RootRunID) == "" || value.RootRunID != strings.TrimSpace(value.RootRunID) {
			return fmt.Errorf("runs: recovery commit Pending deletion[%d] has invalid identity", index)
		}
		key := value.SessionID + "\x00" + value.RootRunID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Pending deletion %q/%q", value.SessionID, value.RootRunID)
		}
		seen[key] = struct{}{}
		owner, found := lostByID[value.RootRunID]
		if !found || !owner.Lineage().IsRoot() || owner.SessionID != value.SessionID {
			return fmt.Errorf(
				"runs: recovery commit Pending deletion %q/%q is not owned by a lost root Run",
				value.SessionID,
				value.RootRunID,
			)
		}
		if index > 0 {
			previous := values[index-1]
			if previous.SessionID > value.SessionID ||
				(previous.SessionID == value.SessionID && previous.RootRunID >= value.RootRunID) {
				return errors.New("runs: recovery commit Pending deletions are not in canonical order")
			}
		}
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
