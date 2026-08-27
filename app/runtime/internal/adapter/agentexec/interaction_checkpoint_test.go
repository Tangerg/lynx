package agentexec

import (
	"encoding/base64"
	"reflect"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

func TestInteractionPendingSteersRoundTripCanonicalContent(t *testing.T) {
	firstID, err := agent.ParseSignalID("steer:02")
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := agent.ParseSignalID("steer:01")
	if err != nil {
		t.Fatal(err)
	}
	pending := map[agent.SignalID]pendingInteractionSteer{
		firstID: {content: []transcript.ContentBlock{{
			Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{0, 1, 2},
		}}},
		secondID: {content: []transcript.ContentBlock{{
			Kind: transcript.TextContent, Text: "revise",
		}}},
	}
	wire, err := encodeInteractionPendingSteers(pending)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 || wire[0].SignalID != secondID.String() || wire[1].SignalID != firstID.String() {
		t.Fatalf("pending steer order = %#v", wire)
	}
	decoded, err := decodeInteractionPendingSteers(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, pending) {
		t.Fatalf("decoded pending steers = %#v, want %#v", decoded, pending)
	}
}

func TestDecodeInteractionPendingSteersRejectsNoncanonicalWire(t *testing.T) {
	validImage := base64.StdEncoding.EncodeToString([]byte{1})
	tests := map[string][]interactionPendingSteerWire{
		"unordered": {
			{SignalID: "steer:02", Content: []interactionContentBlockWire{{Kind: "text", Text: "two"}}},
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
		},
		"duplicate": {
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "again"}}},
		},
		"empty content": {{SignalID: "steer:01"}},
		"mixed text": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "text", Text: "one", MediaType: "text/plain"}},
		}},
		"mixed image": {{
			SignalID: "steer:01",
			Content: []interactionContentBlockWire{{
				Kind: "image", Text: "caption", MediaType: "image/png", Data: validImage,
			}},
		}},
		"non-image media": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "image", MediaType: "text/plain", Data: validImage}},
		}},
		"invalid base64": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "image", MediaType: "image/png", Data: "*"}},
		}},
		"unknown kind": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "audio", Data: validImage}},
		}},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeInteractionPendingSteers(values); err == nil {
				t.Fatal("decode succeeded")
			}
		})
	}
}

func TestDecodeInteractionCheckpointRejectsPreviousSchema(t *testing.T) {
	if _, err := decodeInteractionCheckpointPayload([]byte(`{"schema_version":2}`)); err == nil {
		t.Fatal("decode accepted previous checkpoint schema")
	}
}
