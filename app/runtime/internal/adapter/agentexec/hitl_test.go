package agentexec

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

func TestValidateInterruptSnapshot(t *testing.T) {
	assistant := chat.NewAssistantMessage(chat.MessageParams{Parts: []chat.OutputPart{
		&chat.ToolCallPart{ID: "call_1", Name: "ask_user", Arguments: `{}`},
	}})
	tail, err := marshalMessages([]chat.Message{assistant})
	if err != nil {
		t.Fatalf("marshal tail: %v", err)
	}
	blackboard, _ := core.TagBlackboard(map[string]any{inflightTailKey: tail}, nil)
	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{Blackboard: blackboard}); err != nil {
		t.Fatalf("ValidateInterruptSnapshot(valid): %v", err)
	}

	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{}); err == nil {
		t.Fatal("ValidateInterruptSnapshot accepted a snapshot without a tail")
	}
	invalid, err := marshalMessages([]chat.Message{chat.NewAssistantMessage("plain text")})
	if err != nil {
		t.Fatalf("marshal invalid tail: %v", err)
	}
	blackboard, _ = core.TagBlackboard(map[string]any{inflightTailKey: invalid}, nil)
	if err := ValidateInterruptSnapshot(core.ProcessSnapshot{Blackboard: blackboard}); err == nil {
		t.Fatal("ValidateInterruptSnapshot accepted a non-tool assistant tail")
	}
}
