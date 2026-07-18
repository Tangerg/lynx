package server

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func validArtifact() protocol.SessionArtifact {
	return protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.Session{ID: "ses_1"},
		Runs: []protocol.ArtifactRun{{
			MessageMark: 0,
			Run: protocol.RunRef{
				ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusFinished,
				Outcome:    &protocol.RunOutcome{Type: protocol.OutcomeCompleted, Result: &protocol.RunResult{}},
				FinishedAt: time.Unix(1, 0),
			},
		}},
		Items: []protocol.ArtifactItem{{Item: protocol.Item{
			ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusCompleted,
			Type:    protocol.ItemTypeUserMessage,
			Content: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hello"}},
		}}},
	}
}

func TestArtifactDecodeRejectsInvalidCurrentShape(t *testing.T) {
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

func TestCanonicalArtifactRoundTripsRunLostProblem(t *testing.T) {
	artifact := validArtifact()
	artifact.Runs[0].Run.Outcome = &protocol.RunOutcome{
		Type: protocol.OutcomeError,
		Result: &protocol.RunResult{Error: &protocol.ProblemData{
			Type: protocol.ProblemRunLost, Channel: protocol.ErrorChannelRun,
			Detail: "run lost on restart",
		}},
	}
	runs, _, err := canonicalArtifact(artifact, 0)
	if err != nil {
		t.Fatalf("canonicalArtifact: %v", err)
	}
	got := presentRun(runs[0])
	if got.Outcome == nil || got.Outcome.Result == nil || got.Outcome.Result.Error == nil || got.Outcome.Result.Error.Type != protocol.ProblemRunLost {
		t.Fatalf("round-tripped run = %+v, want run_lost", got)
	}
}

func TestCanonicalArtifactPreservesNetworkSafetyClass(t *testing.T) {
	artifact := validArtifact()
	item := &artifact.Items[0].Item
	item.Type = protocol.ItemTypeToolCall
	item.Content = nil
	item.Tool = &protocol.ToolInvocation{Name: "webfetch", Arguments: map[string]any{"url": "https://example.com"}}
	item.SafetyClass = protocol.SafetyClassNetwork

	_, items, err := canonicalArtifact(artifact, 0)
	if err != nil {
		t.Fatalf("canonicalArtifact: %v", err)
	}
	if items[0].SafetyClass != tool.SafetyClassNetwork {
		t.Fatalf("canonical safety class = %q, want network", items[0].SafetyClass)
	}
	wire := presentItem(items[0])
	if wire.SafetyClass != protocol.SafetyClassNetwork {
		t.Fatalf("round-tripped safety class = %q, want network", wire.SafetyClass)
	}
}

func TestCanonicalToolResultsRejectsInvalidBindings(t *testing.T) {
	base := validArtifact()
	item := &base.Items[0].Item
	item.Type = protocol.ItemTypeToolCall
	item.Content = nil
	item.Tool = &protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{}, Result: "preview"}
	valid := protocol.ArtifactToolResult{
		ID: "BLOB234", ItemID: item.ID, ToolName: "shell",
		Preview: "preview", Body: "full", CreatedAt: time.Unix(2, 0).UTC(),
	}

	for _, test := range []struct {
		name   string
		mutate func(*protocol.SessionArtifact)
	}{
		{"invalid id", func(a *protocol.SessionArtifact) { a.ToolResults[0].ID = "not-valid" }},
		{"unknown item", func(a *protocol.SessionArtifact) { a.ToolResults[0].ItemID = "missing" }},
		{"wrong tool", func(a *protocol.SessionArtifact) { a.ToolResults[0].ToolName = "write" }},
		{"missing preview", func(a *protocol.SessionArtifact) { a.ToolResults[0].Preview = "" }},
		{"missing body", func(a *protocol.SessionArtifact) { a.ToolResults[0].Body = "" }},
		{"mismatched item preview", func(a *protocol.SessionArtifact) {
			a.Items[0].Item.Tool = &protocol.ToolInvocation{Name: "shell", Arguments: map[string]any{}, Result: "different"}
		}},
		{"duplicate id", func(a *protocol.SessionArtifact) { a.ToolResults = append(a.ToolResults, a.ToolResults[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := base
			artifact.Items = append([]protocol.ArtifactItem(nil), base.Items...)
			artifact.ToolResults = []protocol.ArtifactToolResult{valid}
			test.mutate(&artifact)
			_, items, err := canonicalArtifact(artifact, 0)
			if err != nil {
				t.Fatalf("canonicalArtifact setup: %v", err)
			}
			if _, err := canonicalToolResults(artifact, items); !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("canonicalToolResults error = %v, want ErrInvalidParams", err)
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
