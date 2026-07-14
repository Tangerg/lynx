package agentexec

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
)

func TestValidateInterruptSnapshot(t *testing.T) {
	definition := chat.ToolDefinition{Name: "ask_user", InputSchema: json.RawMessage(`{"type":"object"}`)}
	request := &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("question"))},
		Tools:    []chat.ToolDefinition{definition},
	}
	assistant := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call_1", Name: "ask_user", Arguments: `{}`}))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &assistant, FinishReason: chat.FinishReasonToolCalls})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &toolloop.Checkpoint{ID: "approval", Round: 1, Request: request, Response: response}
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	blackboard, _ := core.TagBlackboard(map[string]any{checkpointKey: string(encoded)}, nil)
	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{Blackboard: blackboard}); err != nil {
		t.Fatalf("ValidateInterruptSnapshot(valid): %v", err)
	}

	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{}); err == nil {
		t.Fatal("ValidateInterruptSnapshot accepted a snapshot without a checkpoint")
	}
	blackboard, _ = core.TagBlackboard(map[string]any{checkpointKey: `{"id":"broken"}`}, nil)
	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{Blackboard: blackboard}); err == nil {
		t.Fatal("ValidateInterruptSnapshot accepted an invalid checkpoint")
	}
}
