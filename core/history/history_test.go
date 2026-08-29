package history_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/history"
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
