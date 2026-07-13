package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func portableSnapshot() Snapshot {
	outcome := execution.OutcomeCompleted
	return Snapshot{
		Session: session.Session{ID: "ses_1"},
		Runs: []transcript.Run{{
			SessionID: "ses_1", ID: "run_1", State: execution.Completed,
			Outcome: &outcome, Result: &transcript.RunResult{},
			FinishedAt: time.Unix(2, 0), MessageMark: 0,
		}},
		Items: []transcript.Item{{
			SessionID: "ses_1", ID: "item_1", RunID: "run_1",
			Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
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
		{"state outcome mismatch", func(s *Snapshot) { s.Runs[0].State = execution.Failed }},
		{"missing finished time", func(s *Snapshot) { s.Runs[0].FinishedAt = time.Time{} }},
		{"wrong item session", func(s *Snapshot) { s.Items[0].SessionID = "ses_other" }},
		{"duplicate item", func(s *Snapshot) { s.Items = append(s.Items, s.Items[0]) }},
		{"unknown item status", func(s *Snapshot) { s.Items[0].Status = transcript.ItemStatus(255) }},
		{"unknown spawning item", func(s *Snapshot) { s.Runs[0].SpawnedByItemID = "item_missing" }},
		{"non-tool spawning item", func(s *Snapshot) { s.Runs[0].SpawnedByItemID = "item_1" }},
		{"self-spawn", func(s *Snapshot) {
			s.Items[0].Kind = transcript.ToolCall
			s.Runs[0].SpawnedByItemID = "item_1"
		}},
		{"run tree cycle", func(s *Snapshot) {
			child := s.Runs[0]
			child.ID = "run_2"
			child.SpawnedByItemID = "item_1"
			s.Runs = append(s.Runs, child)
			s.Items[0].Kind = transcript.ToolCall
			s.Items = append(s.Items, transcript.Item{
				SessionID: "ses_1", ID: "item_2", RunID: "run_2",
				Status: transcript.ItemCompleted, Kind: transcript.ToolCall,
			})
			s.Runs[0].SpawnedByItemID = "item_2"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := portableSnapshot()
			test.mutate(&snapshot)
			if err := validateSnapshot(snapshot); err == nil {
				t.Fatal("validateSnapshot accepted inconsistent state")
			}
		})
	}
}

func TestValidateSnapshotAcceptsCanonicalTerminalState(t *testing.T) {
	if err := validateSnapshot(portableSnapshot()); err != nil {
		t.Fatalf("validateSnapshot: %v", err)
	}
}
