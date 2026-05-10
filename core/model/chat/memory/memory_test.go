package memory_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

func TestInMemoryStore_WriteRead(t *testing.T) {
	store := memory.NewInMemoryStore()
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
	store := memory.NewInMemoryStore()
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
	store := memory.NewInMemoryStore()
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
	store := memory.NewInMemoryStore()
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
	store := memory.NewInMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
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
	if _, err := memory.NewMessageWindowStore(nil); err == nil {
		t.Fatal("nil storage must error")
	}
}

func TestNewMessageWindowStore_AvoidsDoubleWrap(t *testing.T) {
	store := memory.NewInMemoryStore()
	wrapped, _ := memory.NewMessageWindowStore(store, 20)

	again, err := memory.NewMessageWindowStore(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if again != wrapped {
		t.Fatal("re-wrapping a MessageWindowStore should return the same instance")
	}
}

func TestMessageWindowStore_Read_KeepsRecentNonSystem(t *testing.T) {
	base := memory.NewInMemoryStore()
	windowed, _ := memory.NewMessageWindowStore(base, 10)
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
	base := memory.NewInMemoryStore()
	windowed, _ := memory.NewMessageWindowStore(base, 10)
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

func TestMemoryMiddleware_RejectsNilStore(t *testing.T) {
	if _, _, err := memory.NewMiddleware(nil); err == nil {
		t.Fatal("nil store must error")
	}
}
