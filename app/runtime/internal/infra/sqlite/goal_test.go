package sqlite_test

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

func newGoalStore(t *testing.T) (*sqlite.GoalStore, *sqlite.SessionStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewGoalStore(db), sqlite.NewSessionStore(db)
}

func TestGoalStoreRecordRunIsIdempotentAndBlocksAtBudget(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "ses_goal_run"
	seedSession(t, sessions, sessionID)
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	g, err := goal.New(sessionID, "finish", modelref.Selection{}, goal.Budget{MaxRuns: 1}, "lease_goal_run", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, err := store.Save(t.Context(), g, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save = (%v, %v), want true, nil", applied, err)
	}
	record := goal.RunRecord{
		SessionID: sessionID, IncarnationID: g.IncarnationID, RunID: "run_goal_run",
		Outcome: run.OutcomeCompleted, CostUSD: 0.25, Steps: 3, CompletedAt: now.Add(time.Minute),
	}
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("repeat RecordRun: %v", err)
	}
	conflict := record
	conflict.IncarnationID = "another_lease"
	if err := store.RecordRun(t.Context(), conflict); !errors.Is(err, goal.ErrRunIdentityConflict) {
		t.Fatalf("conflicting RecordRun = %v, want ErrRunIdentityConflict", err)
	}
	got, found, err := store.Get(t.Context(), sessionID)
	if err != nil || !found {
		t.Fatalf("Get = (%v, %v), want found", found, err)
	}
	if got.Used != (goal.Usage{Runs: 1, CostUSD: 0.25, Steps: 3}) || got.Status != goal.StatusBlocked || got.Reason.Code != goal.ReasonRunBudgetReached {
		t.Fatalf("goal after idempotent RecordRun = %+v", got)
	}
}

func TestGoalSchemaUsesSemanticIncarnationColumns(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	columnsOf := func(table string) []string {
		t.Helper()
		rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		defer rows.Close()
		var columns []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			columns = append(columns, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		return columns
	}

	for _, test := range []struct {
		table    string
		current  string
		obsolete string
	}{
		{table: "goals", current: "incarnation_id", obsolete: "lease_id"},
		{table: "goal_runs", current: "incarnation_id", obsolete: "lease_id"},
		{table: "runs", current: "goal_incarnation_id", obsolete: "goal_lease_id"},
		{table: "interrupts", current: "goal_incarnation_id", obsolete: "goal_lease_id"},
	} {
		columns := columnsOf(test.table)
		if !slices.Contains(columns, test.current) {
			t.Errorf("%s columns = %v, want %s", test.table, columns, test.current)
		}
		if slices.Contains(columns, test.obsolete) {
			t.Errorf("%s columns retain obsolete %s: %v", test.table, test.obsolete, columns)
		}
	}

	goalColumns := columnsOf("goals")
	if !slices.Contains(goalColumns, "reason_code") {
		t.Errorf("goals columns = %v, want reason_code", goalColumns)
	}
	if slices.Contains(goalColumns, "reason_cause") {
		t.Errorf("goals columns retain obsolete reason_cause: %v", goalColumns)
	}
}

func seedSession(t *testing.T, store *sqlite.SessionStore, id string) {
	t.Helper()
	value := sessionfixture.MustRestore(session.Snapshot{ID: id, CWD: "/work"})
	if err := store.Insert(t.Context(), value); err != nil {
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
	g, err := goal.New(sess, "ship the feature", testModelSelection(t, "anthropic", "claude"), goal.Budget{MaxRuns: 5, MaxCostUSD: 2.5}, "lease-round-trip", now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g.AddRun(0.4, 3, now)
	if _, applied, err := store.Save(ctx, g, goal.Version{}); err != nil || !applied {
		t.Fatalf("Save: applied=%v err=%v", applied, err)
	}

	got, ok, err := store.Get(ctx, sess)
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want (true, nil)", ok, err)
	}
	if got.Objective != "ship the feature" || got.Status != goal.StatusActive ||
		got.Budget.MaxRuns != 5 || got.Budget.MaxCostUSD != 2.5 ||
		got.Used.Runs != 1 || got.Used.CostUSD != 0.4 || got.Used.Steps != 3 ||
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
		g, _ := goal.New(s, "obj-"+s, modelref.Selection{}, goal.Budget{}, "lease-"+s, now)
		if _, applied, err := store.Save(ctx, g, goal.Version{}); err != nil || !applied {
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

	mk := func(incarnationID string, revision int64, status goal.Status) goal.Goal {
		g, _ := goal.New(sess, "obj", modelref.Selection{}, goal.Budget{}, incarnationID, now)
		g.IncarnationID = incarnationID
		g.Revision = revision
		g.Status = status
		return g
	}

	initial := mk("lease-one", 1, goal.StatusActive)
	// The zero version inserts when absent, then refuses a second insert.
	if _, applied, err := store.Save(ctx, initial, goal.Version{}); err != nil || !applied {
		t.Fatalf("insert: applied=%v err=%v", applied, err)
	}
	if _, applied, _ := store.Save(ctx, initial, goal.Version{}); applied {
		t.Fatal("zero version must not overwrite an existing goal")
	}

	// A stale writer (zero expectation, wrong incarnation, or wrong revision) is rejected — no
	// clobber, no resurrection.
	if _, applied, _ := store.Save(ctx, mk("lease-two", 99, goal.StatusPaused), goal.Version{IncarnationID: "lease-one", Revision: 99}); applied {
		t.Fatal("mismatched revision must not apply")
	}
	// A lifecycle transition preserves the objective incarnation while the store
	// advances the revision.
	paused := initial
	paused.Pause(goal.ReasonStoppedByUser, "", now)
	var applied bool
	var err error
	if paused, applied, err = store.Save(ctx, paused, initial.Version()); err != nil || !applied {
		t.Fatalf("cas update: applied=%v err=%v", applied, err)
	}
	got, _, _ := store.Get(ctx, sess)
	if got.Version() != paused.Version() || got.Status != goal.StatusPaused {
		t.Fatalf("after cas: version=%+v status=%q, want %+v/paused", got.Version(), got.Status, paused.Version())
	}

	// A same-incarnation mutation advances revision and rejects the prior revision.
	blocked := paused
	blocked.Block(goal.ReasonRunBudgetReached, "", now)
	if blocked, applied, err = store.Save(ctx, blocked, paused.Version()); err != nil || !applied {
		t.Fatalf("same-incarnation update: applied=%v err=%v", applied, err)
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

func TestGoalStoreReplacesExistingGoalWithoutCallerRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	first, _ := goal.New(sessionID, "first", modelref.Selection{}, goal.Budget{}, "lease-first", now)
	first, applied, err := store.Save(t.Context(), first, goal.Version{})
	if err != nil || !applied {
		t.Fatalf("insert first goal: applied=%v err=%v", applied, err)
	}
	firstVersion := first.Version()
	first.Pause(goal.ReasonStoppedByUser, "", now.Add(time.Second))
	first, applied, err = store.Save(t.Context(), first, firstVersion)
	if err != nil || !applied {
		t.Fatalf("stop first goal: applied=%v err=%v", applied, err)
	}

	fresh, _ := goal.New(sessionID, "second", modelref.Selection{}, goal.Budget{}, "lease-second", now.Add(2*time.Second))
	fresh, applied, err = store.Save(t.Context(), fresh, first.Version())
	if err != nil || !applied {
		t.Fatalf("replace goal: applied=%v err=%v", applied, err)
	}
	if fresh.Revision != first.Revision+1 || fresh.Objective != "second" || fresh.IncarnationID != "lease-second" {
		t.Fatalf("replacement = %+v, previous = %+v", fresh, first)
	}
}

func TestGoalStore_ClearThenRecreateRejectsStaleIncarnation(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	now := time.Unix(1_700_000_000, 0).UTC()

	stale, _ := goal.New(sessionID, "old", modelref.Selection{}, goal.Budget{}, "lease-old", now)
	if _, applied, err := store.Save(t.Context(), stale, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed stale goal: applied=%v err=%v", applied, err)
	}
	if err := store.Clear(t.Context(), sessionID); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	fresh, _ := goal.New(sessionID, "new", modelref.Selection{}, goal.Budget{}, "lease-fresh", now)
	if _, applied, err := store.Save(t.Context(), fresh, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed fresh goal: applied=%v err=%v", applied, err)
	}

	stale.Pause(goal.ReasonRunNotCompleted, "error", now)
	if _, applied, err := store.Save(t.Context(), stale, goal.Version{IncarnationID: "lease-old", Revision: 1}); err != nil || applied {
		t.Fatalf("stale Save: applied=%v err=%v, want false/nil", applied, err)
	}
	if applied, err := store.ClearIf(t.Context(), sessionID, goal.Version{IncarnationID: "lease-old", Revision: 1}); err != nil || applied {
		t.Fatalf("stale ClearIf: applied=%v err=%v, want false/nil", applied, err)
	}
	got, ok, err := store.Get(t.Context(), sessionID)
	if err != nil || !ok || got.Objective != "new" || got.IncarnationID != "lease-fresh" {
		t.Fatalf("fresh goal was changed: goal=%+v present=%v err=%v", got, ok, err)
	}
}

// TestGoalStoreRejectsMissingSession is the lifecycle boundary's evidence for
// goal_never_outlives_its_session: the CAS that opens a goal cannot open one for a
// session that is not there, so no lifecycle transition can resurrect a goal whose
// session has already gone.
func TestGoalStoreRejectsMissingSession(t *testing.T) {
	store, _ := newGoalStore(t)
	g, _ := goal.New("missing", "obj", modelref.Selection{}, goal.Budget{}, "lease-missing", time.Unix(0, 0))
	if _, applied, err := store.Save(t.Context(), g, goal.Version{}); err == nil || applied {
		t.Fatalf("Save(missing session) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}

func TestGoalStoreCascadesWithSessionDeletion(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", modelref.Selection{}, goal.Budget{}, "lease", time.Unix(0, 0))
	if _, applied, err := store.Save(t.Context(), g, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed goal: applied=%v err=%v", applied, err)
	}
	record := goal.RunRecord{
		SessionID: sessionID, IncarnationID: g.IncarnationID, RunID: "run-reusable-after-delete",
		Outcome: run.OutcomeCompleted, CompletedAt: time.Unix(1, 0),
	}
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("record old Goal Run: %v", err)
	}

	if err := sessions.Delete(t.Context(), sessionID); err != nil {
		t.Fatalf("Delete(session): %v", err)
	}
	if _, ok, err := store.Get(t.Context(), sessionID); err != nil || ok {
		t.Fatalf("goal after session delete = present=%v err=%v, want false/nil", ok, err)
	}

	// Reusing the same ids proves the old idempotency ledger row was owned by and
	// cascaded with the deleted Session.
	seedSession(t, sessions, sessionID)
	recreated, _ := goal.New(sessionID, "new", modelref.Selection{}, goal.Budget{}, "lease-new", time.Unix(2, 0))
	if _, applied, err := store.Save(t.Context(), recreated, goal.Version{}); err != nil || !applied {
		t.Fatalf("seed recreated goal: applied=%v err=%v", applied, err)
	}
	record.IncarnationID = recreated.IncarnationID
	record.CompletedAt = time.Unix(3, 0)
	if err := store.RecordRun(t.Context(), record); err != nil {
		t.Fatalf("reuse terminal identity after session deletion: %v", err)
	}
	got, ok, err := store.Get(t.Context(), sessionID)
	if err != nil || !ok || got.Used.Runs != 1 {
		t.Fatalf("recreated goal accounting = %+v, present=%v err=%v", got.Used, ok, err)
	}
}

func TestGoalStoreOwnsRevision(t *testing.T) {
	store, sessions := newGoalStore(t)
	const sessionID = "s"
	seedSession(t, sessions, sessionID)
	g, _ := goal.New(sessionID, "obj", modelref.Selection{}, goal.Budget{}, "lease", time.Unix(0, 0))
	g.Revision = 99
	saved, applied, err := store.Save(t.Context(), g, goal.Version{})
	if err != nil || !applied || saved.Revision != 1 {
		t.Fatalf("insert = revision %d, applied=%v err=%v, want 1/true/nil", saved.Revision, applied, err)
	}

	updated := saved
	updated.Pause(goal.ReasonStoppedByUser, "", time.Unix(2, 0))
	updated.Revision = 99
	updated, applied, err = store.Save(t.Context(), updated, saved.Version())
	if err != nil || !applied || updated.Revision != 2 {
		t.Fatalf("update = revision %d, applied=%v err=%v, want 2/true/nil", updated.Revision, applied, err)
	}

	if _, applied, err := store.Save(t.Context(), updated, goal.Version{Revision: 2}); err == nil || applied {
		t.Fatalf("Save(invalid expected incarnation) = applied=%v err=%v, want false/non-nil", applied, err)
	}
	if _, applied, err := store.Save(t.Context(), updated, goal.Version{IncarnationID: updated.IncarnationID, Revision: math.MaxInt64}); err == nil || applied {
		t.Fatalf("Save(exhausted revision) = applied=%v err=%v, want false/non-nil", applied, err)
	}
}
