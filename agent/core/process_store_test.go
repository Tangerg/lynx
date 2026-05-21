package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestInMemoryProcessStore_SaveLoad(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	ctx := context.Background()

	snap := core.ProcessSnapshot{
		ID:        "p-1",
		AgentName: "demo",
		StartedAt: time.Now().Add(-time.Hour),
		Status:    core.StatusRunning,
		Cost:      0.012,
		Tokens:    1500,
	}
	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load(ctx, "p-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != "p-1" || got.AgentName != "demo" || got.Tokens != 1500 {
		t.Errorf("round-trip mismatch: %#v", got)
	}
}

func TestInMemoryProcessStore_LoadMissing(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	_, err := store.Load(context.Background(), "ghost")
	if !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Errorf("want ErrSnapshotNotFound, got %v", err)
	}
}

func TestInMemoryProcessStore_SaveEmptyID(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	err := store.Save(context.Background(), core.ProcessSnapshot{AgentName: "x"})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestInMemoryProcessStore_Delete(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	ctx := context.Background()

	if err := store.Save(ctx, core.ProcessSnapshot{ID: "p-1", AgentName: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "p-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Load(ctx, "p-1"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Errorf("post-delete load: want NotFound, got %v", err)
	}

	// Delete of unknown id is a no-op.
	if err := store.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("delete unknown: %v", err)
	}
}

func TestInMemoryProcessStore_List(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		if err := store.Save(ctx, core.ProcessSnapshot{ID: id, AgentName: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("list: want 3 ids, got %d", len(ids))
	}
}

func TestInMemoryProcessStore_SaveOverwrites(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	ctx := context.Background()

	_ = store.Save(ctx, core.ProcessSnapshot{ID: "p-1", AgentName: "x", Tokens: 100})
	_ = store.Save(ctx, core.ProcessSnapshot{ID: "p-1", AgentName: "x", Tokens: 200})

	got, _ := store.Load(ctx, "p-1")
	if got.Tokens != 200 {
		t.Errorf("overwrite: want 200, got %d", got.Tokens)
	}
}
