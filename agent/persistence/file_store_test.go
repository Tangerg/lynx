package persistence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/persistence"
)

func TestFileStore_RoundTrip(t *testing.T) {
	store, err := persistence.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	snap := core.ProcessSnapshot{
		ID:         "proc-1",
		AgentName:  "researcher",
		Status:     core.StatusWaiting,
		StartedAt:  time.Unix(1700000000, 0).UTC(),
		CapturedAt: time.Unix(1700000100, 0).UTC(),
		Cost:       0.42,
		Tokens:     1234,
		Conditions: map[string]bool{"approved": true},
		Blackboard: map[string]any{"topic": "go generics"},
	}

	if err := store.Save(ctx, snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, "proc-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != snap.ID || got.AgentName != snap.AgentName || got.Status != snap.Status {
		t.Fatalf("round-trip mismatch: %#v", got)
	}
	if got.Cost != 0.42 || got.Tokens != 1234 {
		t.Fatalf("budget mismatch: cost=%v tokens=%v", got.Cost, got.Tokens)
	}
	if !got.Conditions["approved"] {
		t.Fatalf("conditions lost: %#v", got.Conditions)
	}
}

func TestFileStore_CompositeIDsAndList(t *testing.T) {
	store, err := persistence.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Composite child ids (slashes, spaces) must survive the filename
	// round-trip.
	ids := []string{"root", "root >> child/1", "root >> child 2"}
	for _, id := range ids {
		if err := store.Save(ctx, core.ProcessSnapshot{ID: id, AgentName: "a"}); err != nil {
			t.Fatalf("Save %q: %v", id, err)
		}
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != len(ids) {
		t.Fatalf("List = %v, want %d ids", listed, len(ids))
	}
	for _, id := range ids {
		if _, err := store.Load(ctx, id); err != nil {
			t.Errorf("Load %q after List: %v", id, err)
		}
	}
}

func TestFileStore_NotFoundAndDelete(t *testing.T) {
	store, err := persistence.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := store.Load(ctx, "ghost"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load unknown: want ErrSnapshotNotFound, got %v", err)
	}

	// Delete of unknown id is idempotent.
	if err := store.Delete(ctx, "ghost"); err != nil {
		t.Fatalf("Delete unknown: %v", err)
	}

	_ = store.Save(ctx, core.ProcessSnapshot{ID: "p", AgentName: "a"})
	if err := store.Delete(ctx, "p"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Load(ctx, "p"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("after Delete: want ErrSnapshotNotFound, got %v", err)
	}
}
