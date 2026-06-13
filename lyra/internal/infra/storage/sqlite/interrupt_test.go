package sqlite_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/infra/storage/sqlite"
)

func newInterruptStore(t *testing.T) *sqlite.InterruptStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewInterruptStore(db)
}

func TestInterruptStore_PutGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	p := interrupts.Pending{
		ParentRunID: "run_1",
		SessionID:   "ses_a",
		TurnID:      "turn_1",
		Interrupts:  json.RawMessage(`[{"kind":"plan"}]`),
		CreatedAt:   time.Unix(5, 0).UTC(),
	}
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// UPSERT overwrite.
	p.SessionID = "ses_b"
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.SessionID != "ses_b" || string(got.Interrupts) != `[{"kind":"plan"}]` || !got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
		t.Fatalf("Get returned %+v", got)
	}

	if list, _ := store.List(ctx, "ses_b"); len(list) != 1 {
		t.Fatalf("List(ses_b) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "nope"); len(list) != 0 {
		t.Fatalf("List(nope) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete not idempotent: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "run_1"); ok {
		t.Fatal("Get after Delete: still present")
	}
}
