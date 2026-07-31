package chathistory_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestNewWindowStoreValidatesConstruction(t *testing.T) {
	if _, err := chathistory.NewWindowStore(nil, 1); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil store error = %v", err)
	}
	var typedNil *basicStore
	if _, err := chathistory.NewWindowStore(typedNil, 1); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("typed-nil store error = %v", err)
	}
	if _, err := chathistory.NewWindowStore(chathistory.NewInMemoryStore(), 0); !errors.Is(err, chathistory.ErrInvalidWindow) {
		t.Fatalf("invalid limit error = %v", err)
	}
}

func TestWindowStoreMergesSystemAndKeepsRecentMessages(t *testing.T) {
	base := chathistory.NewInMemoryStore()
	firstSystem := chat.NewSystemMessage("first")
	firstSystem.Metadata = metadata.Map{}
	if err := firstSystem.Metadata.Set("shared", "first"); err != nil {
		t.Fatal(err)
	}
	secondSystem := chat.NewSystemMessage("second")
	secondSystem.Metadata = metadata.Map{}
	if err := secondSystem.Metadata.Set("shared", "second"); err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{firstSystem, chat.NewUserMessage(chat.NewTextPart("one")), secondSystem}
	for _, text := range []string{"two", "three", "four"} {
		messages = append(messages, chat.NewUserMessage(chat.NewTextPart(text)))
	}
	if err := base.Write(t.Context(), "c", messages...); err != nil {
		t.Fatal(err)
	}
	window, err := chathistory.NewWindowStore(base, 3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := window.Read(t.Context(), "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Role != chat.RoleSystem || got[0].Text() != "first\n\nsecond" || got[1].Text() != "three" || got[2].Text() != "four" {
		t.Fatalf("window = %#v", got)
	}
	if value := string(got[0].Metadata["shared"]); value != `"second"` {
		t.Fatalf("merged metadata = %s", value)
	}
}

func TestWindowStorePreservesSystemPartStructure(t *testing.T) {
	base := chathistory.NewInMemoryStore()
	part := chat.NewTextPart("")
	part.Metadata = metadata.Map{}
	if err := part.Metadata.Set("provider", "preserved"); err != nil {
		t.Fatal(err)
	}
	first := chat.Message{Role: chat.RoleSystem, Parts: []chat.Part{chat.NewTextPart("first"), part}}
	second := chat.NewSystemMessage("second")
	if err := base.Write(t.Context(), "c", first, second); err != nil {
		t.Fatal(err)
	}
	window, err := chathistory.NewWindowStore(base, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := window.Read(t.Context(), "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Parts) != 4 {
		t.Fatalf("merged messages = %#v", got)
	}
	if value := string(got[0].Parts[1].Metadata["provider"]); value != `"preserved"` {
		t.Fatalf("part metadata = %s", value)
	}
}

func TestWindowStoreDelegatesWritesAndClear(t *testing.T) {
	base := chathistory.NewInMemoryStore()
	window, err := chathistory.NewWindowStore(base, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := window.Write(t.Context(), "b", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if err := window.Write(t.Context(), "a", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatal(err)
	}
	if err := window.Clear(t.Context(), "a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := window.Read(t.Context(), "a"); got == nil || len(got) != 0 {
		t.Fatalf("after Clear = %#v", got)
	}
}
