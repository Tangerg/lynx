package agentexec

import (
	"encoding/base64"
	"math"
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
		}}, projectedItemID: "item_followup"},
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
	if _, err := decodeInteractionCheckpointPayload([]byte(`{"schema_version":4}`)); err == nil {
		t.Fatal("decode accepted previous checkpoint schema")
	}
}

func TestInteractionModelContextsRoundTripCanonicalCalibration(t *testing.T) {
	first, err := agent.ParseProcessID("process:context-02")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.ParseProcessID("process:context-01")
	if err != nil {
		t.Fatal(err)
	}
	firstCalibration, err := NewModelContextTokenCalibration(12_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	secondCalibration, err := NewModelContextTokenCalibration(8_000, 9_000)
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[agent.ProcessID]ModelContextTokenCalibration{
		first: firstCalibration, second: secondCalibration,
	}

	wire, err := encodeInteractionModelContexts(contexts)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 || wire[0].MemberID != second.String() || wire[1].MemberID != first.String() {
		t.Fatalf("model context order = %#v", wire)
	}
	processes := map[agent.ProcessID]struct{}{first: {}, second: {}}
	calls := map[agent.ProcessID]map[string]int{
		first: {"model": 1}, second: {"model": 1},
	}
	decoded, err := decodeInteractionModelContexts(wire, processes, calls)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, contexts) {
		t.Fatalf("decoded model contexts = %#v, want %#v", decoded, contexts)
	}
}

func TestModelContextTokenCalibrationKeepsExactMaxIntDelta(t *testing.T) {
	calibration, err := NewModelContextTokenCalibration(int64(math.MaxInt), math.MaxInt)
	if err != nil {
		t.Fatal(err)
	}
	if adjustment := calibration.Adjustment(); adjustment != 0 {
		t.Fatalf("exact MaxInt calibration adjustment = %d, want 0", adjustment)
	}
}

func TestDecodeInteractionModelContextsRejectsForeignOrUnaccountedMember(t *testing.T) {
	processID, err := agent.ParseProcessID("process:context")
	if err != nil {
		t.Fatal(err)
	}
	wire := []interactionModelContextWire{{
		MemberID: processID.String(), ReportedTokens: 100, EstimatedTokens: 90,
	}}
	if _, err := decodeInteractionModelContexts(wire, nil, nil); err == nil {
		t.Fatal("foreign model context decoded")
	}
	if _, err := decodeInteractionModelContexts(
		wire,
		map[agent.ProcessID]struct{}{processID: {}},
		nil,
	); err == nil {
		t.Fatal("unaccounted model context decoded")
	}
}
