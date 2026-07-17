package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newToolResultStore(t *testing.T) *sqlite.ToolResultStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewToolResultStore(db)
}

func TestToolResultOffloadRoundTrip(t *testing.T) {
	store := newToolResultStore(t)
	const (
		sessID = "sess-1"
		body   = "the full, oversized tool output that was offloaded"
	)
	id, err := store.Offload(t.Context(), sessID, "shell", body)
	if err != nil {
		t.Fatalf("offload: %v", err)
	}
	if id == "" {
		t.Fatal("offload returned an empty id")
	}

	got, found, err := store.Fetch(t.Context(), sessID, id)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !found {
		t.Fatal("fetch reported the just-offloaded body as missing")
	}
	if got != body {
		t.Fatalf("fetched body = %q, want %q", got, body)
	}
}

func TestToolResultOffloadMintsDistinctIDs(t *testing.T) {
	store := newToolResultStore(t)
	first, err := store.Offload(t.Context(), "s", "shell", "a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Offload(t.Context(), "s", "shell", "b")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("two offloads collided on id %q", first)
	}
}

func TestToolResultFetchIsSessionScoped(t *testing.T) {
	store := newToolResultStore(t)
	id, err := store.Offload(t.Context(), "owner", "shell", "secret")
	if err != nil {
		t.Fatal(err)
	}
	// A different session must not read another session's offloaded body.
	if _, found, err := store.Fetch(t.Context(), "intruder", id); err != nil || found {
		t.Fatalf("cross-session fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultFetchUnknownIDIsRecoverableMiss(t *testing.T) {
	store := newToolResultStore(t)
	if _, found, err := store.Fetch(t.Context(), "s", "does-not-exist"); err != nil || found {
		t.Fatalf("unknown id = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultDropSession(t *testing.T) {
	store := newToolResultStore(t)
	id, err := store.Offload(t.Context(), "doomed", "shell", "body")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DropSession(t.Context(), "doomed"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "doomed", id); err != nil || found {
		t.Fatalf("post-drop fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}
