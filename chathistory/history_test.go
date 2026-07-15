package chathistory_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestConversationIDContextAndValidation(t *testing.T) {
	if id, ok := chathistory.ConversationID(context.Background()); ok || id != "" {
		t.Fatalf("unbound context = %q/%v", id, ok)
	}
	ctx := chathistory.WithConversationID(context.Background(), "conversation-1")
	if id, ok := chathistory.ConversationID(ctx); !ok || id != "conversation-1" {
		t.Fatalf("ConversationID = %q/%v", id, ok)
	}
	ctx = chathistory.WithConversationID(ctx, "")
	if _, ok := chathistory.ConversationID(ctx); ok {
		t.Fatal("empty child ID did not shadow parent")
	}
	for _, id := range []string{"", " padded", "padded ", "\tvalue"} {
		if err := chathistory.ValidateConversationID(id); !errors.Is(err, chathistory.ErrInvalidConversationID) {
			t.Fatalf("ValidateConversationID(%q) = %v", id, err)
		}
	}
	if err := chathistory.ValidateConversationID("opaque/id:1"); err != nil {
		t.Fatal(err)
	}
}

func TestInMemoryStoreSnapshotsEveryMessageReference(t *testing.T) {
	var store chathistory.InMemoryStore
	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := image.Metadata.Set("source", "caller"); err != nil {
		t.Fatal(err)
	}
	user := chat.NewUserMessage(chat.NewMediaPart(image))
	user.Metadata = metadata.New()
	if err := user.Metadata.Set("turn", 1); err != nil {
		t.Fatal(err)
	}
	assistant := chat.NewAssistantMessage(
		chat.NewReasoningPart("think", []byte{4, 5}),
		chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "weather", Arguments: `{}`}),
	)
	tool := chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny"})
	messages := []chat.Message{user, assistant, tool}

	if err := store.Write(t.Context(), "c1", messages...); err != nil {
		t.Fatal(err)
	}
	messages[0].Metadata["turn"][0] = '9'
	messages[0].Parts[0].Media.Source.Bytes[0] = 9
	messages[0].Parts[0].Media.Metadata["source"][1] = 'X'
	messages[1].Parts[0].Signature[0] = 9
	messages[1].Parts[1].ToolCall.Name = "mutated"
	messages[2].Parts[0].ToolResult.Result = "mutated"

	first, err := store.Read(t.Context(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalHistory(t, first)
	first[0].Metadata["turn"][0] = '8'
	first[0].Parts[0].Media.Source.Bytes[0] = 8
	first[1].Parts[0].Signature[0] = 8
	first[1].Parts[1].ToolCall.Name = "read-mutated"
	first[2].Parts[0].ToolResult.Result = "read-mutated"

	second, err := store.Read(t.Context(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalHistory(t, second)
	if count, err := store.Count(t.Context(), "c1"); err != nil || count != 3 {
		t.Fatalf("Count = %d, %v", count, err)
	}
	if ids, err := store.Conversations(t.Context()); err != nil || !reflect.DeepEqual(ids, []string{"c1"}) {
		t.Fatalf("Conversations = %v, %v", ids, err)
	}
}

func TestInMemoryStoreReplaceClearAndUnknownRead(t *testing.T) {
	store := chathistory.NewInMemoryStore()
	if got, err := store.Read(t.Context(), "missing"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("unknown Read = %#v, %v", got, err)
	}
	if err := store.Write(t.Context(), "c", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if err := chathistory.Replace(t.Context(), store, "c", chat.NewUserMessage(chat.NewTextPart("two"))); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(t.Context(), "c")
	if len(got) != 1 || got[0].Text() != "two" {
		t.Fatalf("after Replace = %#v", got)
	}
	if err := chathistory.Replace(t.Context(), store, "c"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Read(t.Context(), "c"); len(got) != 0 {
		t.Fatalf("after empty Replace = %#v", got)
	}
	if err := store.Clear(t.Context(), "missing"); err != nil {
		t.Fatal(err)
	}
}

func TestInMemoryStoreRejectsInvalidInputAndCancellation(t *testing.T) {
	store := chathistory.NewInMemoryStore()
	if err := store.Write(t.Context(), "", chat.NewUserMessage(chat.NewTextPart("hello"))); !errors.Is(err, chathistory.ErrInvalidConversationID) {
		t.Fatalf("invalid ID error = %v", err)
	}
	if err := store.Write(t.Context(), "c", chat.Message{}); err == nil {
		t.Fatal("invalid message was accepted")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.Write(ctx, "c", chat.NewUserMessage(chat.NewTextPart("hello"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write cancellation = %v", err)
	}
	if _, err := store.Read(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read cancellation = %v", err)
	}
	if err := store.Clear(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Clear cancellation = %v", err)
	}
	if err := store.Replace(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Replace cancellation = %v", err)
	}
	if _, err := store.Count(ctx, "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Count cancellation = %v", err)
	}
	if _, err := store.Conversations(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Conversations cancellation = %v", err)
	}
}

func TestHistoryHelpersUseOptionalCapabilitiesAndFallbacks(t *testing.T) {
	if err := chathistory.Replace(t.Context(), nil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil Replace error = %v", err)
	}
	if _, err := chathistory.Count(t.Context(), nil, "c"); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil Count error = %v", err)
	}

	store := &basicStore{}
	if err := store.Write(t.Context(), "c", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if count, err := chathistory.Count(t.Context(), store, "c"); err != nil || count != 1 {
		t.Fatalf("fallback Count = %d, %v", count, err)
	}
	if err := chathistory.Replace(t.Context(), store, "c", chat.NewUserMessage(chat.NewTextPart("two"))); err != nil {
		t.Fatal(err)
	}
	if store.clears != 1 || len(store.messages) != 1 || store.messages[0].Text() != "two" {
		t.Fatalf("fallback Replace state = clears %d, messages %#v", store.clears, store.messages)
	}
	if err := chathistory.Replace(t.Context(), store, "c"); err != nil || store.clears != 2 {
		t.Fatalf("fallback empty Replace = clears %d, error %v", store.clears, err)
	}
}

func assertOriginalHistory(t *testing.T, messages []chat.Message) {
	t.Helper()
	if len(messages) != 3 {
		t.Fatalf("messages len = %d", len(messages))
	}
	if got := string(messages[0].Metadata["turn"]); got != "1" {
		t.Fatalf("message metadata = %s", got)
	}
	if got := messages[0].Parts[0].Media.Source.Bytes; !reflect.DeepEqual(got, []byte{1, 2, 3}) {
		t.Fatalf("media bytes = %v", got)
	}
	if got := string(messages[0].Parts[0].Media.Metadata["source"]); got != `"caller"` {
		t.Fatalf("media metadata = %s", got)
	}
	if got := messages[1].Parts[0].Signature; !reflect.DeepEqual(got, []byte{4, 5}) {
		t.Fatalf("signature = %v", got)
	}
	if got := messages[1].Parts[1].ToolCall.Name; got != "weather" {
		t.Fatalf("tool call name = %q", got)
	}
	if got := messages[2].Parts[0].ToolResult.Result; got != "sunny" {
		t.Fatalf("tool result = %q", got)
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
