package sqlite_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newTodoStore(t *testing.T) *sqlite.TodoStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewTodoStore(db)
}

func TestTodoStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newTodoStore(t)
	const sess = "session-x"

	// Unknown session → empty, not an error.
	got, err := store.List(ctx, sess)
	if err != nil {
		t.Fatalf("List(empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List(empty) = %v, want none", got)
	}

	want := []todo.Item{
		{Content: "plan", Status: todo.StatusCompleted},
		{Content: "build", Status: todo.StatusInProgress, NextAction: "run focused test"},
		{Content: "ship", Status: todo.StatusPending, BlockedReason: "waiting on release notes"},
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
	if err := store.Replace(ctx, sess, []todo.Item{{Content: "done", Status: todo.StatusCompleted}}); err != nil {
		t.Fatalf("Replace(shrink): %v", err)
	}
	got, _ = store.List(ctx, sess)
	if len(got) != 1 || got[0].Content != "done" {
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
	if err := store.Replace(ctx, "other", []todo.Item{{Content: "x", Status: todo.StatusPending}}); err != nil {
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

// newTodoBoundaryStores pairs the task list with the Run lifecycle, because a
// boundary is recorded by a Run ending: the two stores share one database so the
// terminal transition and the list it captures are the same transaction's work.
func newTodoBoundaryStores(t *testing.T) (*sqlite.TodoStore, *sqlite.RunStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewTodoStore(db), sqlite.NewRunStore(db)
}

// TestTodoBoundaryIsRecordedByTheRunThatEnds is the whole reason this table
// exists: the live projection keeps one value and no history, so "the list as of
// run X" is only answerable if the Run's end recorded it. Each boundary is frozen
// at that moment — a later replacement does not rewrite what an earlier Run saw.
func TestTodoBoundaryIsRecordedByTheRunThatEnds(t *testing.T) {
	ctx := t.Context()
	todos, runs := newTodoBoundaryStores(t)

	first := []todo.Item{{Content: "survey the code", Status: todo.StatusCompleted}}
	if err := todos.Replace(ctx, "ses_A", first); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit run_1: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize run_1: %v", err)
	}

	second := append(slices.Clone(first), todo.Item{Content: "write the fix", Status: todo.StatusInProgress})
	if err := todos.Replace(ctx, "ses_A", second); err != nil {
		t.Fatalf("Replace(grow): %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_2", "ses_A")); err != nil {
		t.Fatalf("admit run_2: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_2", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize run_2: %v", err)
	}

	got, recorded, err := todos.Boundary(ctx, "run_1")
	if err != nil || !recorded {
		t.Fatalf("Boundary(run_1) = %v, recorded %v, err %v", got, recorded, err)
	}
	if len(got) != 1 || got[0].Content != "survey the code" {
		t.Fatalf("Boundary(run_1) = %+v, want the list as it stood then", got)
	}
	got, recorded, err = todos.Boundary(ctx, "run_2")
	if err != nil || !recorded || len(got) != 2 {
		t.Fatalf("Boundary(run_2) = %+v, recorded %v, err %v, want both items", got, recorded, err)
	}
}

// TestTodoBoundaryDistinguishesEmptyFromUnrecorded is the distinction a rollback
// turns into behavior: a Run that ended before the session had any list recorded
// an EMPTY one (rolling back there clears the list), while a Run that never
// recorded a boundary — imported, or already dropped — leaves the caller nothing
// to restore and must not be read as empty.
func TestTodoBoundaryDistinguishesEmptyFromUnrecorded(t *testing.T) {
	ctx := t.Context()
	todos, runs := newTodoBoundaryStores(t)

	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	got, recorded, err := todos.Boundary(ctx, "run_1")
	if err != nil || !recorded || len(got) != 0 {
		t.Fatalf("Boundary(list never written) = %+v, recorded %v, err %v, want a recorded empty list", got, recorded, err)
	}

	if _, recorded, err = todos.Boundary(ctx, "run_never_seen"); err != nil || recorded {
		t.Fatalf("Boundary(unknown run) recorded %v, err %v, want not recorded", recorded, err)
	}

	// An imported Run finished in another runtime: stamping the importing session's
	// live list would invent a boundary that Run never had.
	if err := runs.Restore(ctx, finishedRun("run_imported", "ses_B", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, recorded, err = todos.Boundary(ctx, "run_imported"); err != nil || recorded {
		t.Fatalf("Boundary(imported run) recorded %v, err %v, want not recorded", recorded, err)
	}
}

// TestTodoBoundaryDiesWithItsRun: the boundary's lifecycle is the Run's, enforced
// by the schema rather than by every write-set remembering — a rollback that drops
// a Run cannot leave a boundary addressing a Run that no longer exists.
func TestTodoBoundaryDiesWithItsRun(t *testing.T) {
	ctx := t.Context()
	todos, runs := newTodoBoundaryStores(t)

	if err := todos.Replace(ctx, "ses_A", []todo.Item{{Content: "dropped work", Status: todo.StatusPending}}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if err := runs.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := runs.Terminalize(ctx, finishedRun("run_1", "ses_A", execution.OutcomeCompleted)); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if _, recorded, err := todos.Boundary(ctx, "run_1"); err != nil || !recorded {
		t.Fatalf("Boundary before drop recorded %v, err %v", recorded, err)
	}

	if err := runs.Delete(ctx, "ses_A", "run_1"); err != nil {
		t.Fatalf("delete run: %v", err)
	}
	if _, recorded, err := todos.Boundary(ctx, "run_1"); err != nil || recorded {
		t.Fatalf("Boundary after the Run was dropped recorded %v, err %v, want gone", recorded, err)
	}
}
