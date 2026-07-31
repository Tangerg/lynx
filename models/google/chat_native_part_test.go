package google

import (
	"bytes"
	"testing"

	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
)

func TestNativePartRoundTripPreservesThoughtSignaturePosition(t *testing.T) {
	tests := []struct {
		name string
		part *genai.Part
		kind corechat.PartKind
	}{
		{
			name: "function call",
			part: &genai.Part{
				FunctionCall:     &genai.FunctionCall{Name: "lookup", Args: map[string]any{"id": float64(7)}},
				ThoughtSignature: []byte("signed-tool-state"),
			},
			kind: corechat.PartToolCall,
		},
		{
			name: "answer text",
			part: &genai.Part{
				Text:             "answer",
				ThoughtSignature: []byte("signed-answer-state"),
			},
			kind: corechat.PartText,
		},
		{
			name: "empty signed part",
			part: &genai.Part{
				ThoughtSignature: []byte("signed-empty-state"),
			},
			kind: corechat.PartText,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corePart, err := mapProtocolCandidatePart(0, 3, tt.part)
			if err != nil {
				t.Fatalf("map response part: %v", err)
			}
			if corePart.Kind != tt.kind {
				t.Fatalf("Core kind = %q, want %q", corePart.Kind, tt.kind)
			}

			replayed, err := mapProtocolAssistantParts([]corechat.Part{corePart})
			if err != nil {
				t.Fatalf("map request part: %v", err)
			}
			if len(replayed) != 1 {
				t.Fatalf("replayed parts = %d, want 1", len(replayed))
			}
			if !bytes.Equal(replayed[0].ThoughtSignature, tt.part.ThoughtSignature) {
				t.Fatalf("thought signature = %q, want %q", replayed[0].ThoughtSignature, tt.part.ThoughtSignature)
			}
			if (replayed[0].FunctionCall == nil) != (tt.part.FunctionCall == nil) || replayed[0].Text != tt.part.Text {
				t.Fatalf("replayed part = %#v, want %#v", replayed[0], tt.part)
			}
		})
	}
}
