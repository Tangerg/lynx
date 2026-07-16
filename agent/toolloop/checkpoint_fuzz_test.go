package toolloop

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func FuzzCheckpointJSON(f *testing.F) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("inspect")))
	if err != nil {
		f.Fatal(err)
	}
	request.Tools = []chat.ToolDefinition{{
		Name:        "lookup",
		Description: "look up deployment state",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	digest, err := toolsetDigest(request.Tools)
	if err != nil {
		f.Fatal(err)
	}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
		ID: "call-1", Name: "lookup", Arguments: `{}`,
	}))
	response, err := chat.NewResponse(chat.Choice{
		Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
	})
	if err != nil {
		f.Fatal(err)
	}
	valid, err := json.Marshal(Checkpoint{
		SchemaVersion:      CheckpointSchemaVersion,
		ID:                 "approval-1",
		Round:              1,
		MaxRounds:          4,
		MaxConcurrentCalls: DefaultMaxConcurrentCalls,
		ToolsetDigest:      digest,
		Request:            request,
		Response:           response,
		CallStates: []CallCheckpoint{{
			Status: CallPaused,
			Pending: &PendingCall{
				ID:           "approval-1",
				Reason:       "approval required",
				Prompt:       json.RawMessage(`"approve?"`),
				ResumeSchema: json.RawMessage(`{"type":"string"}`),
			},
		}},
		NextResult: 0,
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{valid, []byte(`{}`), []byte(`{"schema_version":999}`), []byte(`null`)} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var first Checkpoint
		if err := json.Unmarshal(data, &first); err != nil {
			return
		}
		if err := first.Validate(); err != nil {
			t.Fatalf("successful Unmarshal produced invalid Checkpoint: %v", err)
		}
		firstWire, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("Marshal after successful Unmarshal: %v", err)
		}
		var second Checkpoint
		if err := json.Unmarshal(firstWire, &second); err != nil {
			t.Fatalf("Unmarshal canonical Checkpoint: %v", err)
		}
		secondWire, err := json.Marshal(second)
		if err != nil {
			t.Fatalf("Marshal second Checkpoint: %v", err)
		}
		if !bytes.Equal(firstWire, secondWire) {
			t.Fatalf("Checkpoint wire did not reach fixed point: first=%s second=%s", firstWire, secondWire)
		}
	})
}
