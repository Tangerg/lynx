package history_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"
)

func TestInMemoryStore_WriteRead(t *testing.T) {
	store := history.NewInMemoryStore()
	ctx := context.Background()

	if err := store.Write(ctx, "c1", chat.NewUserMessage("hi")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
}

func TestInMemoryStore_Read_UnknownIDReturnsEmptySlice(t *testing.T) {
	store := history.NewInMemoryStore()
	got, err := store.Read(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("Read on unknown id should return non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestInMemoryStore_Read_ReturnsCopy(t *testing.T) {
	store := history.NewInMemoryStore()
	ctx := context.Background()
	_ = store.Write(ctx, "c", chat.NewUserMessage("hi"))

	got1, _ := store.Read(ctx, "c")
	got1[0] = nil // mutate the returned slice

	got2, _ := store.Read(ctx, "c")
	if got2[0] == nil {
		t.Fatal("Read returned the underlying slice; mutations leaked")
	}
}

func TestInMemoryStore_Clear(t *testing.T) {
	store := history.NewInMemoryStore()
	ctx := context.Background()
	_ = store.Write(ctx, "c", chat.NewUserMessage("hi"))

	if err := store.Clear(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Read(ctx, "c")
	if len(got) != 0 {
		t.Fatalf("after Clear, len = %d, want 0", len(got))
	}
}

func TestInMemoryStore_RespectsCancelledContext(t *testing.T) {
	store := history.NewInMemoryStore()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := store.Write(ctx, "c", chat.NewUserMessage("hi")); err == nil {
		t.Fatal("Write must observe ctx cancellation")
	}
	if _, err := store.Read(ctx, "c"); err == nil {
		t.Fatal("Read must observe ctx cancellation")
	}
	if err := store.Clear(ctx, "c"); err == nil {
		t.Fatal("Clear must observe ctx cancellation")
	}
}

func TestNewMessageWindowStore_RejectsNilStorage(t *testing.T) {
	if _, err := history.NewMessageWindowStore(nil); err == nil {
		t.Fatal("nil storage must error")
	}
}

func TestNewMessageWindowStore_AvoidsDoubleWrap(t *testing.T) {
	store := history.NewInMemoryStore()
	wrapped, _ := history.NewMessageWindowStore(store, 20)

	again, err := history.NewMessageWindowStore(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if again != wrapped {
		t.Fatal("re-wrapping a MessageWindowStore should return the same instance")
	}
}

func TestMessageWindowStore_Read_KeepsRecentNonSystem(t *testing.T) {
	base := history.NewInMemoryStore()
	windowed, _ := history.NewMessageWindowStore(base, 10)
	ctx := context.Background()

	// Write 15 user messages — only the most-recent 10 should come back.
	for range 15 {
		_ = base.Write(ctx, "c", chat.NewUserMessage("u"))
	}
	got, err := windowed.Read(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("windowed Read len = %d, want 10", len(got))
	}
}

func TestMessageWindowStore_Read_PrependsMergedSystemMessage(t *testing.T) {
	base := history.NewInMemoryStore()
	windowed, _ := history.NewMessageWindowStore(base, 10)
	ctx := context.Background()

	_ = base.Write(ctx, "c",
		chat.NewSystemMessage("rule1"),
		chat.NewSystemMessage("rule2"),
		chat.NewUserMessage("hi"),
	)

	got, _ := windowed.Read(ctx, "c")
	if got[0].Type() != chat.MessageTypeSystem {
		t.Fatalf("first message type = %s, want system", got[0].Type())
	}
}

func TestInMemoryStore_Conversations(t *testing.T) {
	store := history.NewInMemoryStore()
	ctx := context.Background()

	if ids, err := store.Conversations(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("empty store: ids=%v err=%v, want empty", ids, err)
	}

	_ = store.Write(ctx, "a", chat.NewUserMessage("hi"))
	_ = store.Write(ctx, "b", chat.NewUserMessage("yo"))

	ids, err := store.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != 2 || !got["a"] || !got["b"] {
		t.Fatalf("Conversations = %v, want {a, b}", ids)
	}

	_ = store.Clear(ctx, "a")
	ids, _ = store.Conversations(ctx)
	if len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("after Clear(a), Conversations = %v, want [b]", ids)
	}
}

func TestMessageWindowStore_ConversationsForwards(t *testing.T) {
	base := history.NewInMemoryStore()
	windowed, _ := history.NewMessageWindowStore(base, 10)
	ctx := context.Background()

	_ = base.Write(ctx, "c", chat.NewUserMessage("hi"))

	ids, err := windowed.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "c" {
		t.Fatalf("windowed.Conversations = %v, want [c]", ids)
	}
}
