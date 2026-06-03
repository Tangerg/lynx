package storage_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

func TestFileProcessStore_SaveLoadRoundTrip(t *testing.T) {
	withTempHome(t)

	store, err := storage.NewFileProcessStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	snap := core.ProcessSnapshot{
		ID:         "proc-1",
		AgentName:  "chat",
		Status:     core.StatusWaiting,
		CapturedAt: time.Unix(0, 0).UTC(),
		Blackboard: map[string]any{"plan": "1. do the thing"},
		Conditions: map[string]bool{"lyra.approval.abc": true},
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, "proc-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != "proc-1" || got.AgentName != "chat" || got.Status != core.StatusWaiting {
		t.Fatalf("Load returned %+v", got)
	}
	if got.Conditions["lyra.approval.abc"] != true {
		t.Errorf("condition not round-tripped: %+v", got.Conditions)
	}
}

func TestFileProcessStore_LoadMissingIsSentinel(t *testing.T) {
	withTempHome(t)
	store, _ := storage.NewFileProcessStore()
	if _, err := store.Load(context.Background(), "nope"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load(missing) err = %v, want ErrSnapshotNotFound", err)
	}
}

func TestFileProcessStore_ListAndDelete(t *testing.T) {
	withTempHome(t)
	store, _ := storage.NewFileProcessStore()
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := store.Save(ctx, core.ProcessSnapshot{ID: id}); err != nil {
			t.Fatalf("Save(%s): %v", id, err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"a", "b", "c"}) {
		t.Fatalf("List = %v, want [a b c]", ids)
	}

	if err := store.Delete(ctx, "b"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "b"); err != nil {
		t.Fatalf("Delete is not idempotent: %v", err)
	}
	ids, _ = store.List(ctx)
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"a", "c"}) {
		t.Fatalf("after delete List = %v, want [a c]", ids)
	}
}

func TestFileProcessStore_RejectsUnsafeID(t *testing.T) {
	withTempHome(t)
	store, _ := storage.NewFileProcessStore()
	if err := store.Save(context.Background(), core.ProcessSnapshot{ID: "../escape"}); err == nil {
		t.Fatal("Save with path-traversal id should error")
	}
}
