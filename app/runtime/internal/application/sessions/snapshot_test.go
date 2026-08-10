package sessions

import (
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
)

func portableSnapshot() Snapshot {
	return Snapshot{
		Session: session.Session{ID: "ses_1"},
		Runs: []run.Run{runfixture.MustRestore(run.Snapshot{
			SessionID: "ses_1", ID: "run_1", State: run.Completed,
			Capabilities: run.Capabilities{ChildRuns: true},
			CreatedAt:    time.Unix(1, 0), FinishedAt: time.Unix(2, 0), MessageMark: 0,
		})},
		Items: []transcript.Item{{
			SessionID: "ses_1", ID: "item_1", RunID: "run_1",
			Status: transcript.ItemCompleted, Kind: transcript.UserMessage, OccurredAt: time.Unix(1, 0),
		}},
	}
}

func TestValidateSnapshotRejectsInconsistentPortableState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"duplicate run", func(s *Snapshot) { s.Runs = append(s.Runs, s.Runs[0]) }},
		{"wrong item session", func(s *Snapshot) { s.Items[0].SessionID = "ses_other" }},
		{"duplicate item", func(s *Snapshot) { s.Items = append(s.Items, s.Items[0]) }},
		{"unknown item status", func(s *Snapshot) { s.Items[0].Status = transcript.ItemStatus(255) }},
		{"unknown spawning item", func(s *Snapshot) {
			appendRootedSnapshotRun(s, "run_2", "run_1", "item_missing")
		}},
		{"non-tool spawning item", func(s *Snapshot) {
			appendRootedSnapshotRun(s, "run_2", "run_1", "item_1")
		}},
		{"run tree cycle", func(s *Snapshot) {
			appendRootedSnapshotRun(s, "run_2", "run_3", "item_2")
			appendRootedSnapshotRun(s, "run_3", "run_2", "item_3")
			s.Items = append(s.Items,
				transcript.Item{
					SessionID: "ses_1", ID: "item_2", RunID: "run_3",
					Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
				},
				transcript.Item{
					SessionID: "ses_1", ID: "item_3", RunID: "run_2",
					Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
				},
			)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := portableSnapshot()
			test.mutate(&snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatal("validateSnapshot accepted inconsistent state")
			}
		})
	}
}

func appendRootedSnapshotRun(snapshot *Snapshot, runID, parentRunID, spawningItemID string) {
	child := snapshot.Runs[0].Snapshot()
	child.ID = runID
	child.Lineage = run.Lineage{SpawnedByItemID: spawningItemID, ParentRunID: parentRunID, RootRunID: "run_1"}
	snapshot.Runs = append(snapshot.Runs, runfixture.MustRestore(child))
}

func TestValidateSnapshotAcceptsCanonicalTerminalState(t *testing.T) {
	if err := portableSnapshot().Validate(); err != nil {
		t.Fatalf("validateSnapshot: %v", err)
	}
}

func TestRestorePlanOrdersRunTreeParentsBeforeChildren(t *testing.T) {
	snapshot := portableSnapshot()
	rootSnapshot := snapshot.Runs[0].Snapshot()
	rootSnapshot.ID = "run_root"
	root := runfixture.MustRestore(rootSnapshot)
	childSnapshot := rootSnapshot
	childSnapshot.ID = "run_child"
	childSnapshot.Lineage = run.Lineage{SpawnedByItemID: "item_root_task", ParentRunID: "run_root", RootRunID: "run_root"}
	child := runfixture.MustRestore(childSnapshot)
	grandchildSnapshot := rootSnapshot
	grandchildSnapshot.ID = "run_grandchild"
	grandchildSnapshot.Lineage = run.Lineage{SpawnedByItemID: "item_child_task", ParentRunID: "run_child", RootRunID: "run_root"}
	grandchild := runfixture.MustRestore(grandchildSnapshot)
	snapshot.Runs = []run.Run{grandchild, child, root}

	plan := restorePlan(snapshot)
	got := make([]string, 0, len(plan.Runs))
	for _, run := range plan.Runs {
		got = append(got, run.ID())
	}
	if !slices.Equal(got, []string{"run_root", "run_child", "run_grandchild"}) {
		t.Fatalf("restore run order = %v, want parent-first", got)
	}
	if snapshot.Runs[0].ID() != "run_grandchild" {
		t.Fatal("restorePlan mutated the source snapshot order")
	}
}
