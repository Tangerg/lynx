package history_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/history"
)

func TestConversationIDContextAndValidation(t *testing.T) {
	if id, ok := history.ConversationIDFromContext(t.Context()); ok || id != "" {
		t.Fatalf("unbound context = %q/%v", id, ok)
	}
	ctx := history.WithConversationID(t.Context(), "conversation-1")
	if id, ok := history.ConversationIDFromContext(ctx); !ok || id != "conversation-1" {
		t.Fatalf("ConversationID = %q/%v", id, ok)
	}
	ctx = history.WithConversationID(ctx, "")
	if _, ok := history.ConversationIDFromContext(ctx); ok {
		t.Fatal("empty child ID did not shadow parent")
	}
	for _, id := range []string{"", " padded", "padded ", "\tvalue", "value\x00", string([]byte{0xff})} {
		if _, err := history.NewConversationID(id); !errors.Is(err, history.ErrInvalidConversationID) {
			t.Fatalf("NewConversationID(%q) = %v", id, err)
		}
	}
	conversationID, err := history.NewConversationID("opaque/id:1")
	if err != nil {
		t.Fatal(err)
	}
	if conversationID.String() != "opaque/id:1" {
		t.Fatalf("ConversationID.String() = %q", conversationID.String())
	}
}

func TestHistoryHelpersUseOptionalCapabilities(t *testing.T) {
	if err := history.Replace(t.Context(), nil, "c"); !errors.Is(err, history.ErrNilStore) {
		t.Fatalf("nil Replace error = %v", err)
	}
	if _, err := history.Count(t.Context(), nil, "c"); !errors.Is(err, history.ErrNilStore) {
		t.Fatalf("nil Count error = %v", err)
	}
	var typedNil *basicStore
	if err := history.Replace(t.Context(), typedNil, "c"); !errors.Is(err, history.ErrNilStore) {
		t.Fatalf("typed-nil Replace error = %v", err)
	}
	if _, err := history.Count(t.Context(), typedNil, "c"); !errors.Is(err, history.ErrNilStore) {
		t.Fatalf("typed-nil Count error = %v", err)
	}

	store := &basicStore{}
	if err := store.Write(t.Context(), "c", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if count, err := history.Count(t.Context(), store, "c"); err != nil || count != 1 {
		t.Fatalf("fallback Count = %d, %v", count, err)
	}
	if err := history.Replace(t.Context(), store, "c", chat.NewUserMessage(chat.NewTextPart("two"))); !errors.Is(err, history.ErrReplacementUnsupported) {
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

func (s *basicStore) Write(_ context.Context, _ history.ConversationID, messages ...chat.Message) error {
	s.messages = append(s.messages, messages...)
	return nil
}

func (s *basicStore) Read(context.Context, history.ConversationID) ([]chat.Message, error) {
	return append([]chat.Message(nil), s.messages...), nil
}

func (s *basicStore) Clear(context.Context, history.ConversationID) error {
	s.clears++
	s.messages = nil
	return nil
}
