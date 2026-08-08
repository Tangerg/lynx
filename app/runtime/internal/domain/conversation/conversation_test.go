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
	history, err = history.AppendUser(chat.NewUserMessage(chat.NewTextPart("steer")))
	if err != nil {
		t.Fatal(err)
	}
	messages := history.Messages()
	if len(messages) != 2 || messages[0].Text() != "one" || messages[1].Text() != "steer" {
		t.Fatalf("messages = %#v", messages)
	}
	messages[0] = chat.NewUserMessage(chat.NewTextPart("changed"))
	if history.Messages()[0].Text() != "one" {
		t.Fatal("Messages leaked aggregate ownership")
	}
}

func TestConversationRejectsInvalidAppend(t *testing.T) {
	if _, err := (Conversation{}).AppendUser(chat.NewAssistantMessage(chat.NewTextPart("no"))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-user append error = %v", err)
	}
	if _, err := (Conversation{}).AppendUser(chat.NewUserMessage()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid append error = %v", err)
	}
}
