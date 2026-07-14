package chathistory_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestNewWindowStoreValidatesConstruction(t *testing.T) {
	if _, err := chathistory.NewWindowStore(nil, 1); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("nil store error = %v", err)
	}
	if _, err := chathistory.NewWindowStore(chathistory.NewInMemoryStore(), 0); !errors.Is(err, chathistory.ErrInvalidWindow) {
		t.Fatalf("invalid limit error = %v", err)
	}
}

func TestWindowStoreMergesSystemAndKeepsRecentMessages(t *testing.T) {
	base := chathistory.NewInMemoryStore()
	firstSystem := chat.NewSystemMessage("first")
	firstSystem.Metadata = metadata.New()
	if err := metadata.Set(firstSystem.Metadata, "shared", "first"); err != nil {
		t.Fatal(err)
	}
	secondSystem := chat.NewSystemMessage("second")
	secondSystem.Metadata = metadata.New()
	if err := metadata.Set(secondSystem.Metadata, "shared", "second"); err != nil {
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

func TestWindowStoreDelegatesWritesReplaceClearAndListing(t *testing.T) {
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
	if ids, err := window.Conversations(t.Context()); err != nil || !reflect.DeepEqual(ids, []string{"a", "b"}) {
		t.Fatalf("Conversations = %v, %v", ids, err)
	}
	if err := window.Replace(t.Context(), "a", chat.NewUserMessage(chat.NewTextPart("two"))); err != nil {
		t.Fatal(err)
	}
	if got, _ := base.Read(t.Context(), "a"); len(got) != 1 || got[0].Text() != "two" {
		t.Fatalf("after Replace = %#v", got)
	}
	if err := window.Clear(t.Context(), "a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := window.Read(t.Context(), "a"); got == nil || len(got) != 0 {
		t.Fatalf("after Clear = %#v", got)
	}
}

func TestWindowStoreReportsUnsupportedListing(t *testing.T) {
	window, err := chathistory.NewWindowStore(&basicStore{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := window.Conversations(t.Context()); !errors.Is(err, chathistory.ErrListingUnsupported) {
		t.Fatalf("Conversations error = %v", err)
	}
}
