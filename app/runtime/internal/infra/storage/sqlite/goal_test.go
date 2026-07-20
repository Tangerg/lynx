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
	if err := store.Save(ctx, g); err != nil {
		t.Fatalf("Save: %v", err)
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
		if err := store.Save(ctx, g); err != nil {
			t.Fatalf("Save(%s): %v", s, err)
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
