package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newGoalStore(t *testing.T) (*sqlite.GoalStore, *sqlite.SessionStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewGoalStore(db), sqlite.NewSessionStore(db)
}

func TestGoalStoreRecordTurnIsIdempotentAndBlocksAtBudget(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "ses_goal_turn"
	seedSession(t, sessions, sessionID)
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	g, err := goal.New(sessionID, "finish", modelref.Selection{}, goal.Budget{MaxTurns: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	g.RenewLease("lease_goal_turn")
	if applied, err := store.Save(t.Context(), g, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save = (%v, %v), want true, nil", applied, err)
	}
	record := goal.TurnRecord{
		SessionID: sessionID, LeaseID: g.LeaseID, RunID: "run_goal_turn",
		Outcome: execution.OutcomeCompleted, CostUSD: 0.25, Steps: 3, CompletedAt: now.Add(time.Minute),
	}
	if err := store.RecordTurn(t.Context(), record); err != nil {
		t.Fatalf("RecordTurn: %v", err)
	}
	if err := store.RecordTurn(t.Context(), record); err != nil {
		t.Fatalf("repeat RecordTurn: %v", err)
	}
	got, found, err := store.Get(t.Context(), sessionID)
	if err != nil || !found {
		t.Fatalf("Get = (%v, %v), want found", found, err)
	}
	if got.Used != (goal.Usage{Turns: 1, CostUSD: 0.25, Steps: 3}) || got.Status != goal.StatusBlocked || got.Reason.Cause != goal.ReasonTurnBudgetReached {
		t.Fatalf("goal after idempotent RecordTurn = %+v", got)
	}
}

func seedSession(t *testing.T, store *sqlite.SessionStore, id string) {
	t.Helper()
	if err := store.Restore(t.Context(), session.Session{ID: id, StartedAt: time.Unix(0, 0), UpdatedAt: time.Unix(0, 0), Revision: 1}); err != nil {
		t.Fatalf("seed session %q: %v", id, err)
	}
}

func TestGoalStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	const sess = "sess-goal"
	seedSession(t, sessions, sess)

	if _, ok, err := store.Get(ctx, sess); err != nil || ok {
		t.Fatalf("Get(unknown) = (%v, %v), want (false, nil)", ok, err)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	g, err := goal.New(sess, "ship the feature", testModelSelection(t, "anthropic", "claude"), goal.Budget{MaxTurns: 5, MaxCostUSD: 2.5}, now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g.AddTurn(0.4, 3, now)
	g.RenewLease("lease-round-trip")
	if applied, err := store.Save(ctx, g, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save: applied=%v err=%v", applied, err)
	}

	got, ok, err := store.Get(ctx, sess)
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want (true, nil)", ok, err)
	}
	if got.Objective != "ship the feature" || got.Status != goal.StatusActive ||
		got.Budget.MaxTurns != 5 || got.Budget.MaxCostUSD != 2.5 ||
		got.Used.Turns != 1 || got.Used.CostUSD != 0.4 || got.Used.Steps != 3 ||
		got.ModelSelection.Provider() != "anthropic" || got.ModelSelection.Model() != "claude" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, now)
	}
}

func testModelSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func TestGoalStore_ListAndClear(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	for _, s := range []string{"a", "b"} {
		seedSession(t, sessions, s)
		g, _ := goal.New(s, "obj-"+s, modelref.Selection{}, goal.Budget{}, now)
		g.RenewLease("lease-" + s)
		if applied, err := store.Save(ctx, g, goal.Version{}); err != nil || !applied {
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
// expected zero version, update-if-version-matches otherwise, and reject a stale writer
// (including ClearIf) so a superseded loop can neither clobber a newer goal nor
// resurrect a cleared one.
func TestGoalStore_CompareAndSwap(t *testing.T) {
	ctx := context.Background()
	store, sessions := newGoalStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	const sess = "s"
	seedSession(t, sessions, sess)

	mk := func(leaseID string, revision int64, status goal.Status) goal.Goal {
		g, _ := goal.New(sess, "obj", modelref.Selection{}, goal.Budget{}, now)
		g.LeaseID = leaseID
		g.Revision = revision
		g.Status = status
		return g
	}

	initial := mk("lease-one", 1, goal.StatusActive)
	// The zero version inserts when absent, then refuses a second insert.
	if applied, err := store.Save(ctx, initial, goal.Version{}); err != nil || !applied {
		t.Fatalf("insert: applied=%v err=%v", applied, err)
	}
	if applied, _ := store.Save(ctx, initial, goal.Version{}); applied {
		t.Fatal("zero version must not overwrite an existing goal")
	}

	// A stale writer (zero expectation, wrong lease, or wrong revision) is rejected — no
	// clobber, no resurrection.
	if applied, _ := store.Save(ctx, mk("lease-two", 2, goal.StatusPaused), goal.Version{LeaseID: "lease-one", Revision: 99}); applied {
		t.Fatal("mismatched revision must not apply")
	}
	// A lifecycle transition renews the lease and advances the revision.
	paused := mk("lease-two", 2, goal.StatusActive)
	paused.Pause(goal.ReasonStoppedByUser, "", now)
	if applied, err := store.Save(ctx, paused, initial.Version()); err != nil || !applied {
		t.Fatalf("cas update: applied=%v err=%v", applied, err)
	}
	got, _, _ := store.Get(ctx, sess)
	if got.Version() != paused.Version() || got.Status != goal.StatusPaused {
		t.Fatalf("after cas: version=%+v status=%q, want %+v/paused", got.Version(), got.Status, paused.Version())
	}

	// A same-lease mutation advances revision and rejects the prior revision.
	blocked := paused
	blocked.Block(goal.ReasonTurnBudgetReached, "", now)
	blocked.AdvanceRevision()
	if applied, err := store.Save(ctx, blocked, paused.Version()); err != nil || !applied {
		t.Fatalf("same-lease update: applied=%v err=%v", applied, err)
	}
	if applied, _ := store.ClearIf(ctx, sess, paused.Version()); applied {
		t.Fatal("ClearIf must not delete on a stale revision")
	}
	if applied, err := store.ClearIf(ctx, sess, blocked.Version()); err != nil || !applied {
		t.Fatalf("ClearIf(match): applied=%v err=%v", applied, err)
	}
	if _, ok, _ := store.Get(ctx, sess); ok {
		t.Fatal("goal should be gone after a matching ClearIf")
	}
}

func TestGoalStore_ClearThenRecreateRejectsStaleLease(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	stale, _ := goal.New(sessionID, "old", modelref.Selection{}, goal.Budget{}, now)
	stale.RenewLease("lease-old")
	if applied, err := store.Save(t.Context(), stale, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed stale goal: applied=%v err=%v", applied, err)
	}
	if err := store.Clear(t.Context(), sessionID); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	fresh, _ := goal.New(sessionID, "new", modelref.Selection{}, goal.Budget{}, now)
	fresh.RenewLease("lease-fresh")
	if applied, err := store.Save(t.Context(), fresh, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed fresh goal: applied=%v err=%v", applied, err)
	}

	stale.Pause(goal.ReasonRunNotCompleted, "error", now)
	stale.AdvanceRevision()
	if applied, err := store.Save(t.Context(), stale, goal.Version{LeaseID: "lease-old", Revision: 1}); err != nil || applied {
		t.Fatalf("stale Save: applied=%v err=%v, want false/nil", applied, err)
	}
	if applied, err := store.ClearIf(t.Context(), sessionID, goal.Version{LeaseID: "lease-old", Revision: 1}); err != nil || applied {
		t.Fatalf("stale ClearIf: applied=%v err=%v, want false/nil", applied, err)
	}
	got, ok, err := store.Get(t.Context(), sessionID)
	if err != nil || !ok || got.Objective != "new" || got.LeaseID != "lease-fresh" {
		t.Fatalf("fresh goal was changed: goal=%+v present=%v err=%v", got, ok, err)
	}
}

func TestGoalStoreRejectsMissingSession(t *testing.T) {
	store, _ := newGoalStore(t)
	g, _ := goal.New("missing", "obj", modelref.Selection{}, goal.Budget{}, time.Unix(0, 0))
	g.RenewLease("lease-missing")
	if applied, err := store.Save(t.Context(), g, goal.Version{}); err == nil || applied {
		t.Fatalf("Save(missing session) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}

func TestGoalStoreCascadesWithSessionDeletion(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", modelref.Selection{}, goal.Budget{}, time.Unix(0, 0))
	g.RenewLease("lease")
	if applied, err := store.Save(t.Context(), g, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed goal: applied=%v err=%v", applied, err)
	}

	if err := sessions.Delete(t.Context(), sessionID); err != nil {
		t.Fatalf("Delete(session): %v", err)
	}
	if _, ok, err := store.Get(t.Context(), sessionID); err != nil || ok {
		t.Fatalf("goal after session delete = present=%v err=%v, want false/nil", ok, err)
	}
}

func TestGoalStoreRejectsInvalidVersion(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", modelref.Selection{}, goal.Budget{}, time.Unix(0, 0))

	if applied, err := store.Save(t.Context(), g, goal.Version{}); err == nil || applied {
		t.Fatalf("Save(zero version) = applied=%v err=%v, want false/non-nil", applied, err)
	}
	g.RenewLease("")
	if applied, err := store.Save(t.Context(), g, goal.Version{}); err == nil || applied {
		t.Fatalf("Save(empty lease) = applied=%v err=%v, want false/non-nil", applied, err)
	}
	g.LeaseID = "lease"
	g.Revision = 0
	if applied, err := store.Save(t.Context(), g, goal.Version{}); err == nil || applied {
		t.Fatalf("Save(zero revision) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}
