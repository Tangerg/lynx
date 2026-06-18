package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// TestParkStore_ConsumeIsAtomic pins the HITL-resume idempotency fix: Consume
// reads AND deletes a parked round in one statement (DELETE ... RETURNING), so
// a consumed round can't linger to hijack a later fresh turn on the same
// conversation — the bug a read-then-best-effort-clear had when the clear
// failed. It also exercises that modernc SQLite supports RETURNING.
func TestParkStore_ConsumeIsAtomic(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewParkStore(db)
	ctx := context.Background()

	// Nothing parked yet → (nil, nil).
	if st, err := store.Consume(ctx, "conv-1"); err != nil || st != nil {
		t.Fatalf("Consume(empty) = (%v, %v), want (nil, nil)", st, err)
	}

	if err := store.Write(ctx, "conv-1", &tool.ParkState{
		Assistant: chat.NewAssistantMessage("parked round"),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// First consume returns the round.
	got, err := store.Consume(ctx, "conv-1")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got == nil || got.Assistant == nil {
		t.Fatalf("Consume returned %+v, want the parked round", got)
	}

	// Second consume finds nothing — the round was removed atomically with the
	// read, so it can never re-inject onto a later turn.
	if st, err := store.Consume(ctx, "conv-1"); err != nil || st != nil {
		t.Fatalf("second Consume = (%v, %v), want (nil, nil) — the round must be gone", st, err)
	}
}
