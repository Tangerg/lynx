package chathistory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestConversationIDContextAndValidation(t *testing.T) {
	if id, ok := chathistory.ConversationID(t.Context()); ok || id != "" {
		t.Fatalf("unbound context = %q/%v", id, ok)
	}
	ctx := chathistory.WithConversationID(t.Context(), "conversation-1")
	if id, ok := chathistory.ConversationID(ctx); !ok || id != "conversation-1" {
		t.Fatalf("ConversationID = %q/%v", id, ok)
	}
	ctx = chathistory.WithConversationID(ctx, "")
	if _, ok := chathistory.ConversationID(ctx); ok {
		t.Fatal("empty child ID did not shadow parent")
	}
	for _, id := range []string{"", " padded", "padded ", "\tvalue", "value\x00", string([]byte{0xff})} {
		if err := chathistory.ValidateConversationID(id); !errors.Is(err, chathistory.ErrInvalidConversationID) {
			t.Fatalf("ValidateConversationID(%q) = %v", id, err)
		}
	}
	if err := chathistory.ValidateConversationID("opaque/id:1"); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryHelpersUseOptionalCapabilities(t *testing.T) {
	if err := chathistory.Replace(t.Context(), nil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil Replace error = %v", err)
	}
	if _, err := chathistory.Count(t.Context(), nil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil Count error = %v", err)
	}
	var typedNil *basicStore
	if err := chathistory.Replace(t.Context(), typedNil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("typed-nil Replace error = %v", err)
	}
	if _, err := chathistory.Count(t.Context(), typedNil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("typed-nil Count error = %v", err)
	}

	store := &basicStore{}
	if err := store.Write(t.Context(), "c", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if count, err := chathistory.Count(t.Context(), store, "c"); err != nil || count != 1 {
		t.Fatalf("fallback Count = %d, %v", count, err)
	}
	if err := chathistory.Replace(t.Context(), store, "c", chat.NewUserMessage(chat.NewTextPart("two"))); !errors.Is(err, chathistory.ErrReplacementUnsupported) {
		t.Fatalf("unsupported Replace error = %v", err)
	}
	if store.clears != 0 || len(store.messages) != 1 || store.messages[0].Text() != "one" {
		t.Fatalf("unsupported Replace mutated state = clears %d, messages %#v", store.clears, store.messages)
	}
}

type basicStore struct {
	messages []chat.Message
	clears   int
}

func (s *basicStore) Write(_ context.Context, _ string, messages ...chat.Message) error {
	s.messages = append(s.messages, messages...)
	return nil
}

func (s *basicStore) Read(context.Context, string) ([]chat.Message, error) {
	return append([]chat.Message(nil), s.messages...), nil
}

func (s *basicStore) Clear(context.Context, string) error {
	s.clears++
	s.messages = nil
	return nil
}
