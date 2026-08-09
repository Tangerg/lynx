package conversation

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

func TestConversationOwnsSequenceTransitions(t *testing.T) {
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
	}
	history, err := (Conversation{}).Seed(seed)
	if err != nil {
		t.Fatal(err)
	}
	if history.Count() != 2 {
		t.Fatalf("count = %d, want 2", history.Count())
	}
	if _, err := history.Seed(seed); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("second seed error = %v, want ErrNotEmpty", err)
	}
	history = history.Truncate(1)
	messages := history.Messages()
	if len(messages) != 1 || messages[0].Text() != "one" {
		t.Fatalf("messages = %#v", messages)
	}
	messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
	if history.Messages()[0].Text() != "one" {
		t.Fatal("Messages leaked aggregate ownership")
	}
}
