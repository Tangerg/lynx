package conversations

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/chathistory/inmemory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestMessagesCoordinatesDurableHistory(t *testing.T) {
	messages := NewMessages(inmemory.New())
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); err != nil {
		t.Fatal(err)
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); !errors.Is(err, conversation.ErrNotEmpty) {
		t.Fatalf("second seed error = %v", err)
	}
	if err := messages.Truncate(t.Context(), "ses_1", 2); err != nil {
		t.Fatal(err)
	}
	got, err := messages.Read(t.Context(), "ses_1")
	if err != nil || len(got) != 2 || got[1].Text() != "two" {
		t.Fatalf("Read = %#v, %v", got, err)
	}
}

func TestMessagesRejectsMissingSession(t *testing.T) {
	messages := NewMessages(inmemory.New())
	if _, err := messages.Read(t.Context(), ""); !errors.Is(err, errSessionIDRequired) {
		t.Fatalf("Read error = %v", err)
	}
}
