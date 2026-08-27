package interaction

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestHostFailureSignalModesAreExclusive(t *testing.T) {
	modelHost := signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelResult:   &modelCallResult{HostError: "journal unavailable"},
	}
	if err := modelHost.validate(); err != nil {
		t.Fatalf("model host failure: %v", err)
	}
	response := chat.Response{}
	modelHost.ModelResult.Response = &response
	modelHost.ModelResult.EffectiveMessages = []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("must not accompany host failure")),
	}
	if err := modelHost.validate(); err == nil {
		t.Fatal("model result combined a host failure with a response")
	}

	toolHost := signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolResult:    &toolBatchResult{HostError: "journal unavailable"},
	}
	if err := toolHost.validate(); err != nil {
		t.Fatalf("Tool host failure: %v", err)
	}
	toolHost.ToolResult.Results = []chat.ToolResult{{ID: "call", Name: "tool", Result: "value"}}
	if err := toolHost.validate(); err == nil {
		t.Fatal("Tool result combined a host failure with ordinary results")
	}
}

func TestToolBatchPauseCountDoesNotWrap(t *testing.T) {
	request, err := NewToolInputRequest(
		json.RawMessage(`"provide another value"`),
		json.RawMessage(`{"type":"string"}`),
		json.RawMessage(`{"continuation":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatch := toolBatchDispatch{pauseCount: math.MaxUint32}
	if _, err := dispatch.pause(0, request); err == nil {
		t.Fatal("exhausted Tool input pause count wrapped instead of failing")
	}
	if dispatch.pauseCount != math.MaxUint32 {
		t.Fatalf("pause count changed to %d", dispatch.pauseCount)
	}
}
