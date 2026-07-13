package server

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func validArtifact() protocol.SessionArtifact {
	return protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.Session{ID: "ses_1"},
		Runs: []protocol.ArtifactRun{{
			MessageMark: 0,
			Run: protocol.RunRef{
				ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
				Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}},
			},
		}},
		Items: []protocol.ArtifactItem{{Item: protocol.Item{
			ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusCompleted,
			Type:    protocol.ItemTypeUserMessage,
			Content: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hello"}},
		}}},
	}
}

func TestCanonicalArtifactRejectsInvalidCurrentShape(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*protocol.SessionArtifact)
	}{
		{"unknown run status", func(a *protocol.SessionArtifact) { a.Runs[0].Run.Status = "paused" }},
		{"finished without outcome", func(a *protocol.SessionArtifact) { a.Runs[0].Run.Outcome = nil }},
		{"terminal without result", func(a *protocol.SessionArtifact) { a.Runs[0].Run.Outcome.Result = nil }},
		{"unknown outcome", func(a *protocol.SessionArtifact) {
			a.Runs[0].Run.Outcome.Type = "legacy"
		}},
		{"unknown item status", func(a *protocol.SessionArtifact) { a.Items[0].Item.Status = "done" }},
		{"unknown item type", func(a *protocol.SessionArtifact) { a.Items[0].Item.Type = "legacyMessage" }},
		{"unknown content type", func(a *protocol.SessionArtifact) { a.Items[0].Item.Content[0].Type = "video" }},
		{"content union mismatch", func(a *protocol.SessionArtifact) { a.Items[0].Item.Content[0].Mime = "text/plain" }},
		{"item union mismatch", func(a *protocol.SessionArtifact) { a.Items[0].Item.Text = "not message content" }},
		{"duplicate run id", func(a *protocol.SessionArtifact) { a.Runs = append(a.Runs, a.Runs[0]) }},
		{"duplicate item id", func(a *protocol.SessionArtifact) { a.Items = append(a.Items, a.Items[0]) }},
		{"unknown item run", func(a *protocol.SessionArtifact) { a.Items[0].Item.RunID = "run_missing" }},
		{"invalid message mark", func(a *protocol.SessionArtifact) { a.Runs[0].MessageMark = 1 }},
		{"wrong run session", func(a *protocol.SessionArtifact) { a.Runs[0].Run.SessionID = "ses_other" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := validArtifact()
			test.mutate(&artifact)
			_, _, err := canonicalArtifact(artifact, 0)
			if !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("canonicalArtifact error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestCanonicalArtifactRejectsNonPortableRunStates(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*protocol.SessionArtifact)
	}{
		{"running", func(a *protocol.SessionArtifact) {
			a.Runs[0].Run.Status = protocol.RunStatusRunning
			a.Runs[0].Run.Outcome = nil
		}},
		{"interrupted", func(a *protocol.SessionArtifact) {
			a.Runs[0].Run.Outcome = &protocol.RunOutcome{Type: protocol.OutcomeInterrupt}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := validArtifact()
			test.mutate(&artifact)
			_, _, err := canonicalArtifact(artifact, 0)
			if !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("canonicalArtifact error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestCanonicalArtifactRejectsCyclicRunTree(t *testing.T) {
	tool := func(id, runID string) protocol.ArtifactItem {
		return protocol.ArtifactItem{Item: protocol.Item{
			ID: id, RunID: runID, Status: protocol.ItemStatusCompleted, Type: protocol.ItemTypeToolCall,
			Tool: &protocol.ToolInvocation{Name: "task", Arguments: map[string]any{}},
		}}
	}
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.Session{ID: "ses_1"},
		Runs: []protocol.ArtifactRun{
			{MessageMark: 0, Run: protocol.RunRef{ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}}, SpawnedByItemID: "item_2"}},
			{MessageMark: 0, Run: protocol.RunRef{ID: "run_2", SessionID: "ses_1", Status: protocol.RunStatusFinished, Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}}, SpawnedByItemID: "item_1"}},
		},
		Items: []protocol.ArtifactItem{tool("item_1", "run_1"), tool("item_2", "run_2")},
	}

	_, _, err := canonicalArtifact(artifact, 0)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("canonicalArtifact error = %v, want ErrInvalidParams", err)
	}
}

func FuzzCanonicalArtifactRunStatus(f *testing.F) {
	for _, status := range []string{"", "running", "finished", "paused", "legacy"} {
		f.Add(status)
	}
	f.Fuzz(func(t *testing.T, rawStatus string) {
		artifact := validArtifact()
		artifact.Runs[0].Run.Status = protocol.RunStatus(rawStatus)
		if artifact.Runs[0].Run.Status != protocol.RunStatusFinished {
			artifact.Runs[0].Run.Outcome = nil
		}
		_, _, err := canonicalArtifact(artifact, 0)
		valid := rawStatus == string(protocol.RunStatusFinished)
		if valid && err != nil {
			t.Fatalf("valid status %q rejected: %v", rawStatus, err)
		}
		if !valid && !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("invalid status %q error = %v, want ErrInvalidParams", rawStatus, err)
		}
	})
}
