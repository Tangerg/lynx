package agentexec

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestTrailingUserMessageCountPreservesEveryMessageInOneSteerSignal(t *testing.T) {
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("delegated task")),
		chat.NewAssistantMessage(chat.NewTextPart("working")),
		chat.NewUserMessage(chat.NewTextPart("first steer message")),
		chat.NewUserMessage(chat.NewTextPart("second steer message")),
	}

	if got := trailingUserMessageCount(messages); got != 2 {
		t.Fatalf("trailing User messages = %d, want 2", got)
	}
}
