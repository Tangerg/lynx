package storage_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

func TestFileMessageStore_WriteReadRoundTrip(t *testing.T) {
	withTempHome(t)

	store, err := storage.NewFileMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "conv-1"

	user := chat.NewUserMessage("hello")
	assistant := chat.NewAssistantMessage("hi there")
	if err := store.Write(ctx, id, user, assistant); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := store.Read(ctx, id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Read len = %d, want 2", len(got))
	}
	if got[0].Type() != chat.MessageTypeUser {
		t.Errorf("got[0].Type = %s, want user", got[0].Type())
	}
	if got[1].Type() != chat.MessageTypeAssistant {
		t.Errorf("got[1].Type = %s, want assistant", got[1].Type())
	}
}

// TestFileMessageStore_PersistsAcrossInstances mirrors the session
// test: a freshly-opened store on the same LYRA_HOME must see what
// the previous instance wrote.
func TestFileMessageStore_PersistsAcrossInstances(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()
	const id = "conv-persist"

	first, _ := storage.NewFileMessageStore()
	_ = first.Write(ctx, id, chat.NewUserMessage("turn 1"))

	second, _ := storage.NewFileMessageStore()
	got, err := second.Read(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("Read after restart: got %d msgs", len(got))
	}
}

func TestFileMessageStore_ReadUnknown(t *testing.T) {
	withTempHome(t)
	store, _ := storage.NewFileMessageStore()
	got, err := store.Read(context.Background(), "never-existed")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("unknown conv should read as empty, got %d", len(got))
	}
}

func TestFileMessageStore_ClearIdempotent(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()
	store, _ := storage.NewFileMessageStore()
	const id = "conv-clear"

	_ = store.Write(ctx, id, chat.NewUserMessage("doomed"))
	if err := store.Clear(ctx, id); err != nil {
		t.Fatal(err)
	}
	// Second clear on missing file must not error.
	if err := store.Clear(ctx, id); err != nil {
		t.Errorf("clear idempotent: %v", err)
	}

	got, _ := store.Read(ctx, id)
	if len(got) != 0 {
		t.Errorf("after Clear, Read len = %d", len(got))
	}
}

// TestFileMessageStore_InvalidID rejects ids that could break out
// of the messages directory.
func TestFileMessageStore_InvalidID(t *testing.T) {
	withTempHome(t)
	store, _ := storage.NewFileMessageStore()
	ctx := context.Background()
	for _, id := range []string{"", ".", "..", "a/b", "..\\evil"} {
		if err := store.Write(ctx, id, chat.NewUserMessage("x")); err == nil {
			t.Errorf("Write(%q) accepted; want rejection", id)
		}
		// Read may or may not error depending on platform — at minimum
		// it must not crash. Empty result is acceptable.
		if _, err := store.Read(ctx, id); err == nil {
			if strings.ContainsAny(id, "/\\") {
				t.Errorf("Read(%q) accepted; want rejection", id)
			}
		}
	}
}
