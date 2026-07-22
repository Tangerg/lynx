package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newGoalStore(t *testing.T) *sqlite.GoalStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewGoalStore(db)
}

func TestGoalStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newGoalStore(t)
	const sess = "sess-goal"

	if _, ok, err := store.Get(ctx, sess); err != nil || ok {
		t.Fatalf("Get(unknown) = (%v, %v), want (false, nil)", ok, err)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	g, err := goal.New(sess, "ship the feature", "anthropic", "claude", goal.Budget{MaxTurns: 5, MaxCostUSD: 2.5}, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g.AddTurn(0.4, 3, now)
	g.Generation = 1
	if applied, err := store.Save(ctx, g, 0); err != nil || !applied {
		t.Fatalf("Save: applied=%v err=%v", applied, err)
	}

	got, ok, err := store.Get(ctx, sess)
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want (true, nil)", ok, err)
	}
	if got.Objective != "ship the feature" || got.Status != goal.StatusActive ||
		got.Budget.MaxTurns != 5 || got.Budget.MaxCostUSD != 2.5 ||
		got.Used.Turns != 1 || got.Used.CostUSD != 0.4 || got.Used.Steps != 3 ||
		got.Provider != "anthropic" || got.Model != "claude" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, now)
	}
}

func TestGoalStore_ListAndClear(t *testing.T) {
	ctx := context.Background()
	store := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, s := range []string{"a", "b"} {
		g, _ := goal.New(s, "obj-"+s, "", "", goal.Budget{}, now)
		g.Generation = 1
		if applied, err := store.Save(ctx, g, 0); err != nil || !applied {
			t.Fatalf("Save(%s): applied=%v err=%v", s, applied, err)
		}
	}
	all, err := store.List(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("List = (%d, %v), want 2", len(all), err)
	}

	if err := store.Clear(ctx, "a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "a"); ok {
		t.Fatal("cleared goal still present")
	}
	if _, ok, _ := store.Get(ctx, "b"); !ok {
		t.Fatal("Clear removed the wrong session")
	}
	// Clearing a missing goal is not an error.
	if err := store.Clear(ctx, "missing"); err != nil {
		t.Fatalf("Clear(missing): %v", err)
	}
}

// TestGoalStore_CompareAndSwap covers the keystone CAS: insert-if-absent on
// expected 0, update-if-generation-matches otherwise, and reject a stale writer
// (including ClearIf) so a superseded loop can neither clobber a newer goal nor
// resurrect a cleared one.
func TestGoalStore_CompareAndSwap(t *testing.T) {
	ctx := context.Background()
	store := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	const sess = "s"

	mk := func(gen int64, status goal.Status) goal.Goal {
		g, _ := goal.New(sess, "obj", "", "", goal.Budget{}, now)
		g.Generation = gen
		g.Status = status
		return g
	}

	// expected 0 inserts when absent, then refuses a second insert.
	if applied, err := store.Save(ctx, mk(1, goal.StatusActive), 0); err != nil || !applied {
		t.Fatalf("insert: applied=%v err=%v", applied, err)
	}
	if applied, _ := store.Save(ctx, mk(1, goal.StatusActive), 0); applied {
		t.Fatal("expected 0 must not overwrite an existing goal")
	}

	// A stale writer (expected 0, or a mismatched generation) is rejected — no
	// clobber, no resurrection.
	if applied, _ := store.Save(ctx, mk(2, goal.StatusPaused), 99); applied {
		t.Fatal("mismatched generation must not apply")
	}
	// The current owner (expected 1) advances the goal to generation 2.
	if applied, err := store.Save(ctx, mk(2, goal.StatusPaused), 1); err != nil || !applied {
		t.Fatalf("cas update: applied=%v err=%v", applied, err)
	}
	got, _, _ := store.Get(ctx, sess)
	if got.Generation != 2 || got.Status != goal.StatusPaused {
		t.Fatalf("after cas: gen=%d status=%q, want 2/paused", got.Generation, got.Status)
	}

	// ClearIf rejects a stale generation and applies on a match.
	if applied, _ := store.ClearIf(ctx, sess, 1); applied {
		t.Fatal("ClearIf must not delete on a stale generation")
	}
	if applied, err := store.ClearIf(ctx, sess, 2); err != nil || !applied {
		t.Fatalf("ClearIf(match): applied=%v err=%v", applied, err)
	}
	if _, ok, _ := store.Get(ctx, sess); ok {
		t.Fatal("goal should be gone after a matching ClearIf")
	}
}
