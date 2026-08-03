package sqlite_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newPlanStore(t *testing.T) *sqlite.PlanStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewPlanStore(db)
}

func TestPlanStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newPlanStore(t)
	const sess = "session-x"

	// Unknown session → empty, not an error.
	got, err := store.List(ctx, sess)
	if err != nil {
		t.Fatalf("List(empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List(empty) = %v, want none", got)
	}

	want := []plan.Step{
		{Description: "plan", Status: plan.StatusCompleted},
		{Description: "build", Status: plan.StatusInProgress},
		{Description: "ship", Status: plan.StatusPending},
	}
	if err := store.Replace(ctx, sess, want); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, err = store.List(ctx, sess)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Replace is a full overwrite, not a merge.
	if err := store.Replace(ctx, sess, []plan.Step{{Description: "done", Status: plan.StatusCompleted}}); err != nil {
		t.Fatalf("Replace(shrink): %v", err)
	}
	got, _ = store.List(ctx, sess)
	if len(got) != 1 || got[0].Description != "done" {
		t.Fatalf("after shrink = %v, want single 'done'", got)
	}

	// Clearing to empty round-trips as empty (not NULL).
	if err := store.Replace(ctx, sess, nil); err != nil {
		t.Fatalf("Replace(clear): %v", err)
	}
	got, err = store.List(ctx, sess)
	if err != nil {
		t.Fatalf("List(after clear): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("after clear = %v, want none", got)
	}

	// Lists are per-session.
	if err := store.Replace(ctx, "other", []plan.Step{{Description: "x", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("Replace(other): %v", err)
	}
	if got, _ := store.List(ctx, sess); len(got) != 0 {
		t.Fatalf("session bleed: %v", got)
	}
	if err := store.DeleteSession(ctx, "other"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if got, err := store.List(ctx, "other"); err != nil || len(got) != 0 {
		t.Fatalf("after DeleteSession = %v, %v, want none", got, err)
	}
}

// newPlanBoundaryStores pairs the Plan with the Run lifecycle, because a
// boundary is recorded by a Run ending: the two stores share one database so the
// terminal transition and the list it captures are the same transaction's work.
func newPlanBoundaryStores(t *testing.T) (*sqlite.PlanStore, *sqlite.RunStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewPlanStore(db), sqlite.NewRunStore(db)
}

// TestPlanBoundaryIsRecordedByTheRunThatEnds is the whole reason this table
// exists: the live projection keeps one value and no history, so "the list as of
// run X" is only answerable if the Run's end recorded it. Each boundary is frozen
// at that moment — a later replacement does not rewrite what an earlier Run saw.
func TestPlanBoundaryIsRecordedByTheRunThatEnds(t *testing.T) {
	ctx := t.Context()
	plans, runs := newPlanBoundaryStores(t)

	first := []plan.Step{{Description: "survey the code", Status: plan.StatusCompleted}}
	if err := plans.Replace(ctx, "ses_A", first); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit run_1: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize run_1: %v", err)
	}

	second := append(slices.Clone(first), plan.Step{Description: "write the fix", Status: plan.StatusInProgress})
	if err := plans.Replace(ctx, "ses_A", second); err != nil {
		t.Fatalf("Replace(grow): %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_2", "ses_A")); err != nil {
		t.Fatalf("admit run_2: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_2", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize run_2: %v", err)
	}

	got, recorded, err := plans.Boundary(ctx, "run_1")
	if err != nil || !recorded {
		t.Fatalf("Boundary(run_1) = %v, recorded %v, err %v", got, recorded, err)
	}
	if len(got) != 1 || got[0].Description != "survey the code" {
		t.Fatalf("Boundary(run_1) = %+v, want the list as it stood then", got)
	}
	got, recorded, err = plans.Boundary(ctx, "run_2")
	if err != nil || !recorded || len(got) != 2 {
		t.Fatalf("Boundary(run_2) = %+v, recorded %v, err %v, want both items", got, recorded, err)
	}
}

// TestPlanBoundaryDistinguishesEmptyFromUnrecorded is the distinction a rollback
// turns into behavior: a Run that ended before the session had any list recorded
// an EMPTY one (rolling back there clears the list), while a Run that never
// recorded a boundary — imported, or already dropped — leaves the caller nothing
// to restore and must not be read as empty.
func TestPlanBoundaryDistinguishesEmptyFromUnrecorded(t *testing.T) {
	ctx := t.Context()
	plans, runs := newPlanBoundaryStores(t)

	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	got, recorded, err := plans.Boundary(ctx, "run_1")
	if err != nil || !recorded || len(got) != 0 {
		t.Fatalf("Boundary(list never written) = %+v, recorded %v, err %v, want a recorded empty list", got, recorded, err)
	}

	if _, recorded, err = plans.Boundary(ctx, "run_never_seen"); err != nil || recorded {
		t.Fatalf("Boundary(unknown run) recorded %v, err %v, want not recorded", recorded, err)
	}

	// An imported Run finished in another runtime: stamping the importing session's
	// live list would invent a boundary that Run never had.
	if err := runs.Restore(ctx, finishedRun("run_imported", "ses_B", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, recorded, err = plans.Boundary(ctx, "run_imported"); err != nil || recorded {
		t.Fatalf("Boundary(imported run) recorded %v, err %v, want not recorded", recorded, err)
	}
}

// TestPlanBoundaryDiesWithItsRun: the boundary's lifecycle is the Run's, enforced
// by the schema rather than by every write-set remembering — a rollback that drops
// a Run cannot leave a boundary addressing a Run that no longer exists.
func TestPlanBoundaryDiesWithItsRun(t *testing.T) {
	ctx := t.Context()
	plans, runs := newPlanBoundaryStores(t)

	if err := plans.Replace(ctx, "ses_A", []plan.Step{{Description: "dropped work", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if _, recorded, err := plans.Boundary(ctx, "run_1"); err != nil || !recorded {
		t.Fatalf("Boundary before drop recorded %v, err %v", recorded, err)
	}

	if err := runs.Delete(ctx, "ses_A", "run_1"); err != nil {
		t.Fatalf("delete run: %v", err)
	}
	if _, recorded, err := plans.Boundary(ctx, "run_1"); err != nil || recorded {
		t.Fatalf("Boundary after the Run was dropped recorded %v, err %v, want gone", recorded, err)
	}
}

// TestPlanStateIsOwnedByItsSession proves session_state_is_owned_by_its_session and
// state_revision_never_goes_backwards, the two halves of a session-scoped state key.
//
// The key is declared session-scoped, which means one value per session and one
// revision space per session. Two sessions sharing either would be a panel that
// changes when someone works in another window, or — worse — a client that discards
// its own newer value as stale because the other session's write had already claimed
// that revision number.
//
// The revision half is checked by writing an EARLIER list last: the value goes
// backwards and the revision must not, because a client folds by revision and would
// otherwise treat the restored list as the stale one. That is the shape of the bug
// rollback and import both had.
func TestPlanStateIsOwnedByItsSession(t *testing.T) {
	ctx := context.Background()
	store := newPlanStore(t)

	first := []plan.Step{{Description: "mine", Status: plan.StatusInProgress}}
	if err := store.Replace(ctx, "ses_a", first); err != nil {
		t.Fatalf("replace a: %v", err)
	}
	if err := store.Replace(ctx, "ses_b", []plan.Step{{Description: "theirs", Status: plan.StatusPending}}); err != nil {
		t.Fatalf("replace b: %v", err)
	}

	a, err := store.State(ctx, "ses_a")
	if err != nil {
		t.Fatalf("state a: %v", err)
	}
	b, err := store.State(ctx, "ses_b")
	if err != nil {
		t.Fatalf("state b: %v", err)
	}
	if len(a.Steps) != 1 || a.Steps[0].Description != "mine" {
		t.Fatalf("session a's list = %+v, want only its own item", a.Steps)
	}
	if len(b.Steps) != 1 || b.Steps[0].Description != "theirs" {
		t.Fatalf("session b's list = %+v, want only its own item", b.Steps)
	}
	if a.Revision != b.Revision {
		t.Fatalf("revisions = %d and %d; each session counts its own writes, so one write each is the same number",
			a.Revision, b.Revision)
	}

	// Writing in b again must not move a at all.
	if err := store.Replace(ctx, "ses_b", []plan.Step{{Description: "theirs, again", Status: plan.StatusCompleted}}); err != nil {
		t.Fatalf("replace b again: %v", err)
	}
	unmoved, err := store.State(ctx, "ses_a")
	if err != nil {
		t.Fatalf("re-read a: %v", err)
	}
	if unmoved.Revision != a.Revision || len(unmoved.Steps) != 1 || unmoved.Steps[0].Description != "mine" {
		t.Fatalf("session a moved to %+v because session b was written", unmoved)
	}

	// Restoring an earlier value is a new write, not a return to an old revision.
	if err := store.Replace(ctx, "ses_a", first); err != nil {
		t.Fatalf("restore a: %v", err)
	}
	restored, err := store.State(ctx, "ses_a")
	if err != nil {
		t.Fatalf("read restored a: %v", err)
	}
	if restored.Revision <= a.Revision {
		t.Fatalf("restored revision = %d, want greater than the %d it replaced", restored.Revision, a.Revision)
	}
	if !restored.UpdatedAt.After(a.UpdatedAt) && !restored.UpdatedAt.Equal(a.UpdatedAt) {
		t.Fatalf("restored updatedAt = %s, want no earlier than %s", restored.UpdatedAt, a.UpdatedAt)
	}
}
