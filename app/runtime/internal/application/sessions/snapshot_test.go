package sessions

import (
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func portableSnapshot() Snapshot {
	outcome := run.OutcomeCompleted
	return Snapshot{
		Session: session.Session{ID: "ses_1"},
		Runs: []transcript.Run{{
			SessionID: "ses_1", ID: "run_1", State: run.Completed,
			Outcome:   &outcome,
			CreatedAt: time.Unix(1, 0), FinishedAt: time.Unix(2, 0), MessageMark: 0,
		}},
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
		{"wrong run session", func(s *Snapshot) { s.Runs[0].SessionID = "ses_other" }},
		{"duplicate run", func(s *Snapshot) { s.Runs = append(s.Runs, s.Runs[0]) }},
		{"state outcome mismatch", func(s *Snapshot) { s.Runs[0].State = run.Failed }},
		{"missing finished time", func(s *Snapshot) { s.Runs[0].FinishedAt = time.Time{} }},
		{"missing creation time", func(s *Snapshot) { s.Runs[0].CreatedAt = time.Time{} }},
		{"wrong item session", func(s *Snapshot) { s.Items[0].SessionID = "ses_other" }},
		{"duplicate item", func(s *Snapshot) { s.Items = append(s.Items, s.Items[0]) }},
		{"unknown item status", func(s *Snapshot) { s.Items[0].Status = transcript.ItemStatus(255) }},
		{"partial child lineage", func(s *Snapshot) { s.Runs[0].SpawnedByItemID = "item_missing" }},
		{"unknown spawning item", func(s *Snapshot) {
			appendSnapshotChild(s, "run_2", "run_1", "run_1", "item_missing")
		}},
		{"non-tool spawning item", func(s *Snapshot) {
			appendSnapshotChild(s, "run_2", "run_1", "run_1", "item_1")
		}},
		{"run tree cycle", func(s *Snapshot) {
			appendSnapshotChild(s, "run_2", "run_3", "run_1", "item_2")
			appendSnapshotChild(s, "run_3", "run_2", "run_1", "item_3")
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

func appendSnapshotChild(snapshot *Snapshot, runID, parentRunID, rootRunID, spawningItemID string) {
	child := snapshot.Runs[0]
	child.ID = runID
	child.SpawnedByItemID = spawningItemID
	child.ParentRunID = parentRunID
	child.RootRunID = rootRunID
	snapshot.Runs = append(snapshot.Runs, child)
}

func TestValidateSnapshotAcceptsCanonicalTerminalState(t *testing.T) {
	if err := portableSnapshot().Validate(); err != nil {
		t.Fatalf("validateSnapshot: %v", err)
	}
}

func TestRestorePlanOrdersRunTreeParentsBeforeChildren(t *testing.T) {
	snapshot := portableSnapshot()
	root := snapshot.Runs[0]
	root.ID = "run_root"
	child := root
	child.ID = "run_child"
	child.SpawnedByItemID = "item_root_task"
	child.ParentRunID = "run_root"
	child.RootRunID = "run_root"
	grandchild := root
	grandchild.ID = "run_grandchild"
	grandchild.SpawnedByItemID = "item_child_task"
	grandchild.ParentRunID = "run_child"
	grandchild.RootRunID = "run_root"
	snapshot.Runs = []transcript.Run{grandchild, child, root}

	plan := restorePlan(snapshot)
	got := make([]string, 0, len(plan.Runs))
	for _, run := range plan.Runs {
		got = append(got, run.ID)
	}
	if !slices.Equal(got, []string{"run_root", "run_child", "run_grandchild"}) {
		t.Fatalf("restore run order = %v, want parent-first", got)
	}
	if snapshot.Runs[0].ID != "run_grandchild" {
		t.Fatal("restorePlan mutated the source snapshot order")
	}
}
