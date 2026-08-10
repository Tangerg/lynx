package client

import (
	"strconv"
	"strings"
	"testing"
)

func TestSessionSnapshotValidatesAggregateAndActiveRunTogether(t *testing.T) {
	events := []Envelope{
		snapshotEnvelope(1, "run_1", RunStarted{RunID: "run_1", SessionID: "session_1"}),
		snapshotEnvelope(2, "run_1", RunInterrupted{Interaction: Approval{InterruptID: "approval_1", Title: "Edit"}}),
	}
	snapshot := SessionSnapshot{
		Session: Session{ID: "session_1", Workspace: "/workspace"},
		Events:  events,
		Cursor:  2,
		Active:  &Run{ID: "run_1", SessionID: "session_1", Status: RunWaiting, StartedAfter: 0},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("valid snapshot: %v", err)
	}

	for name, mutate := range map[string]func(*SessionSnapshot){
		"cursor":         func(s *SessionSnapshot) { s.Cursor = 3 },
		"event session":  func(s *SessionSnapshot) { s.Events[1].SessionID = "other" },
		"missing active": func(s *SessionSnapshot) { s.Active = nil },
		"wrong status":   func(s *SessionSnapshot) { s.Active.Status = RunActive },
		"wrong start":    func(s *SessionSnapshot) { s.Active.StartedAfter = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := snapshot
			invalid.Events = append([]Envelope(nil), snapshot.Events...)
			active := *snapshot.Active
			invalid.Active = &active
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("invalid snapshot was accepted")
			}
		})
	}
}

func TestRestoreSnapshotIsAtomic(t *testing.T) {
	conversation := NewConversation()
	valid := SessionSnapshot{
		Session: Session{ID: "session_1", Workspace: "/workspace"},
		Events: []Envelope{
			snapshotEnvelope(1, "run_1", RunStarted{RunID: "run_1", SessionID: "session_1"}),
			snapshotEnvelope(2, "run_1", RunFinished{Outcome: Outcome{Status: OutcomeCompleted}}),
		},
		Cursor: 2,
	}
	if err := conversation.RestoreSnapshot(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Cursor = 3
	if err := conversation.RestoreSnapshot(invalid); err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("invalid restore error = %v", err)
	}
	if conversation.Cursor() != 2 || conversation.Outcome().Status != OutcomeCompleted {
		t.Fatalf("failed restore mutated aggregate: cursor=%d outcome=%+v", conversation.Cursor(), conversation.Outcome())
	}
}

func snapshotEnvelope(cursor Cursor, runID string, event Event) Envelope {
	return Envelope{ID: "event_" + strconv.FormatUint(uint64(cursor), 10), Cursor: cursor, RunID: runID, SessionID: "session_1", Event: event}
}
