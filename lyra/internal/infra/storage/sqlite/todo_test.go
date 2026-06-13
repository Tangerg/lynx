package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/lyra/internal/service/todo"
)

func newTodoStore(t *testing.T) *sqlite.TodoService {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewTodoService(db)
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
		{Content: "build", Status: todo.StatusInProgress},
		{Content: "ship", Status: todo.StatusPending},
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
}
