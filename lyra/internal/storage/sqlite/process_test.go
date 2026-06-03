package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
)

func newProcessStore(t *testing.T) *sqlite.ProcessStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewProcessStore(db)
}

func TestProcessStore_SaveLoadUpsert(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)

	snap := core.ProcessSnapshot{
		ID:         "proc-1",
		AgentName:  "chat",
		Status:     core.StatusWaiting,
		CapturedAt: time.Unix(100, 0).UTC(),
		Conditions: map[string]bool{"k": true},
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Overwrite (UPSERT) — engine auto-snapshots the same id every tick.
	snap.Status = core.StatusCompleted
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("Save overwrite: %v", err)
	}

	got, err := store.Load(ctx, "proc-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Status != core.StatusCompleted || got.Conditions["k"] != true {
		t.Fatalf("Load returned %+v", got)
	}
}

func TestProcessStore_LoadMissingIsSentinel(t *testing.T) {
	store := newProcessStore(t)
	if _, err := store.Load(context.Background(), "nope"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load(missing) err = %v, want ErrSnapshotNotFound", err)
	}
}

func TestProcessStore_ListDeleteIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)

	for _, id := range []string{"a", "b"} {
		if err := store.Save(ctx, core.ProcessSnapshot{ID: id, CapturedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil || len(ids) != 2 {
		t.Fatalf("List = %v (err %v), want 2", ids, err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete not idempotent: %v", err)
	}
	if ids, _ = store.List(ctx); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("after delete List = %v, want [b]", ids)
	}
}
