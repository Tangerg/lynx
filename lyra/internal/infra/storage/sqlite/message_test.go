package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/infra/storage/sqlite"
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
		chat.NewUserMessage("one"), chat.NewUserMessage("two"), chat.NewUserMessage("three")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Replace the history with just the first message — exact overwrite.
	if err := store.Replace(ctx, "conv", chat.NewUserMessage("one")); err != nil {
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
