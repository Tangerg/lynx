package server

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func validArtifact() protocol.SessionArtifact {
	finished := time.Unix(1, 0).UTC()
	return protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{ID: "ses_1"},
		Runs: []protocol.ArtifactRun{{
			ID: "run_1", SessionID: "ses_1", CreatedAt: finished, FinishedAt: finished,
			UpdatedAt: finished, MessageMark: 0,
			Outcome: protocol.ArtifactOutcome{Type: "completed"},
		}},
		Items: []protocol.ArtifactItem{{
			ID: "item_1", RunID: "run_1", Status: "completed", CreatedAt: finished,
			Type: "userMessage", Content: []protocol.ArtifactContentBlock{{Type: "text", Text: "hello"}},
		}},
	}
}

func TestPortableArtifactDecoderRejectsUnknownDiscriminators(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*protocol.SessionArtifact)
	}{
		{"outcome", func(a *protocol.SessionArtifact) { a.Runs[0].Outcome.Type = "legacy" }},
		{"item status", func(a *protocol.SessionArtifact) { a.Items[0].Status = "done" }},
		{"item type", func(a *protocol.SessionArtifact) { a.Items[0].Type = "legacyMessage" }},
		{"content type", func(a *protocol.SessionArtifact) { a.Items[0].Content[0].Type = "video" }},
		{"question field", func(a *protocol.SessionArtifact) {
			a.Items[0].Type = "question"
			a.Items[0].Content = nil
			a.Items[0].Question = &protocol.ArtifactQuestion{Fields: []protocol.ArtifactQuestionField{{Type: "legacy"}}}
		}},
		{"problem type", func(a *protocol.SessionArtifact) {
			a.Runs[0].Outcome.Type = "error"
			a.Runs[0].Outcome.Error = &protocol.ArtifactProblem{Type: "legacy"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := validArtifact()
			test.mutate(&artifact)
			_, err := portableArtifactFromWire(artifact)
			if !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("portableArtifactFromWire error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestPortableArtifactDecoderLeavesAggregateValidationToApplication(t *testing.T) {
	artifact := validArtifact()
	artifact.Runs = append(artifact.Runs, artifact.Runs[0])

	portable, err := portableArtifactFromWire(artifact)
	if err != nil {
		t.Fatalf("portableArtifactFromWire: %v", err)
	}
	if _, err := portable.CanonicalSnapshot(); err == nil {
		t.Fatal("CanonicalSnapshot accepted duplicate run identity")
	}
}

func TestSessionImportMapsApplicationArchiveValidationToInvalidParams(t *testing.T) {
	s, _ := rollbackHarness(t)
	artifact := validArtifact()
	artifact.Runs = append(artifact.Runs, artifact.Runs[0])

	_, err := s.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("ImportSession error = %v, want invalid_params", err)
	}
}

func TestPortableArtifactDecoderPreservesCanonicalToolResult(t *testing.T) {
	artifact := validArtifact()
	at := time.Unix(1, 0).UTC()
	artifact.Items[0] = protocol.ArtifactItem{
		ID: "item_1", RunID: "run_1", Status: "completed", Type: "toolCall",
		CreatedAt: at, StartedAt: at, FinishedAt: at, DurationMs: valuePtr(int64(0)),
		Tool: &protocol.ArtifactToolInvocation{Name: "shell", Arguments: map[string]any{}, Result: map[string]any{"stdout": "raw"}},
	}
	portable, err := portableArtifactFromWire(artifact)
	if err != nil {
		t.Fatalf("portableArtifactFromWire: %v", err)
	}
	result := portable.Items[0].Tool.Result.Any()
	value, ok := result.(map[string]any)
	if !ok || value["stdout"] != "raw" {
		t.Fatalf("tool result = %#v, want the unpresented canonical result", result)
	}
}

func TestPortableArtifactDecoderRejectsInconsistentToolTiming(t *testing.T) {
	startedAt := time.Unix(1, 0).UTC()
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	valid := func() protocol.SessionArtifact {
		artifact := validArtifact()
		artifact.Items[0] = protocol.ArtifactItem{
			ID: "item_1", RunID: "run_1", Status: protocol.ItemStatusCompleted,
			CreatedAt: startedAt, StartedAt: startedAt, FinishedAt: finishedAt,
			DurationMs: valuePtr(int64(1500)), Type: protocol.ItemTypeToolCall,
			Tool: &protocol.ArtifactToolInvocation{Name: "shell", Arguments: map[string]any{}},
		}
		return artifact
	}
	tests := []struct {
		name   string
		mutate func(*protocol.ArtifactItem)
	}{
		{name: "start differs from creation", mutate: func(item *protocol.ArtifactItem) {
			item.StartedAt = item.StartedAt.Add(time.Millisecond)
		}},
		{name: "duration differs from boundaries", mutate: func(item *protocol.ArtifactItem) {
			item.DurationMs = valuePtr(int64(1499))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := valid()
			test.mutate(&artifact.Items[0])
			_, err := portableArtifactFromWire(artifact)
			if !errors.Is(err, protocol.ErrInvalidParams) {
				t.Fatalf("portableArtifactFromWire error = %v, want invalid_params", err)
			}
		})
	}
}
