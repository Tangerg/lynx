package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

// TestMessageStore_ReplaceIsTransactional pins the retention-safety fix:
// Replace sets a conversation's history to exactly the given messages in one
// transaction (DELETE + INSERT), so truncate/compaction can't leave the
// conversation wiped if the rewrite fails. Append (Write) accumulates; Replace
// overwrites; empty clears.
func TestMessageStore_ReplaceIsTransactional(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	if err := store.Write(ctx, "conv",
		chat.NewUserMessage(chat.NewTextPart("one")), chat.NewUserMessage(chat.NewTextPart("two")), chat.NewUserMessage(chat.NewTextPart("three"))); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Replace the history with just the first message — exact overwrite.
	if err := store.Replace(ctx, "conv", chat.NewUserMessage(chat.NewTextPart("one"))); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, err := store.Read(ctx, "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("after Replace len = %d, want 1 (overwrite, not append)", len(got))
	}

	// Replace with nothing clears it.
	if err := store.Replace(ctx, "conv"); err != nil {
		t.Fatalf("Replace empty: %v", err)
	}
	if got, _ := store.Read(ctx, "conv"); len(got) != 0 {
		t.Fatalf("after empty Replace len = %d, want 0", len(got))
	}
}

// TestMessageStore_CountMatchesReadLength pins the Counter capability: Count
// returns the stored message count via COUNT(*) — equal to len(Read) — so a
// watermark read doesn't load and unmarshal the whole history. Unknown
// conversation is 0, not an error.
func TestMessageStore_CountMatchesReadLength(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	if n, err := store.Count(ctx, "conv"); err != nil || n != 0 {
		t.Fatalf("Count of empty = (%d, %v), want (0, nil)", n, err)
	}

	if err := store.Write(ctx, "conv",
		chat.NewUserMessage(chat.NewTextPart("one")), chat.NewUserMessage(chat.NewTextPart("two")), chat.NewUserMessage(chat.NewTextPart("three"))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read(ctx, "conv")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	n, err := store.Count(ctx, "conv")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != len(got) || n != 3 {
		t.Fatalf("Count = %d, len(Read) = %d, want both 3", n, len(got))
	}
}
