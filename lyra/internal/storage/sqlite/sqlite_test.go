package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
)

func newTempDB(t *testing.T) (*sqlite.SessionService, *sqlite.MemoryService) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewSessionService(db), sqlite.NewMemoryService(db)
}

// TestSessionCRUD exercises the full mutate / read cycle of session.Service
// against the SQLite backend.
func TestSessionCRUD(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTempDB(t)

	// empty list at startup
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List on empty DB = %d entries", len(list))
	}

	// create
	created, err := svc.Create(ctx, "first session", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("Create returned empty ID")
	}

	// get
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "first session" {
		t.Fatalf("Get title = %q", got.Title)
	}
	if !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("UpdatedAt round-trip mismatch: got %v want %v", got.UpdatedAt, created.UpdatedAt)
	}

	// list now has one
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List = %+v", list)
	}

	// delete
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// idempotent delete
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete idempotent: %v", err)
	}

	// get after delete
	if _, err := svc.Get(ctx, created.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

// TestSessionFork confirms a child session is linked to its parent and
// metadata records the fork-at-message-id.
func TestSessionFork(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTempDB(t)

	parent, _ := svc.Create(ctx, "parent", "")

	child, err := svc.Fork(ctx, parent.ID, "msg-7")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("child.ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if got := child.Metadata["fork_at_message_id"]; got != "msg-7" {
		t.Fatalf("metadata fork_at_message_id = %q", got)
	}
	if child.Title != "parent (fork)" {
		t.Fatalf("child title = %q", child.Title)
	}

	// fork of unknown parent → ErrNotFound
	if _, err := svc.Fork(ctx, "nope", "msg-0"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Fork unknown parent = %v, want ErrNotFound", err)
	}

	// child round-trips through Get + retains metadata
	gotChild, err := svc.Get(ctx, child.ID)
	if err != nil {
		t.Fatalf("Get child: %v", err)
	}
	if gotChild.Metadata["fork_at_message_id"] != "msg-7" {
		t.Fatalf("metadata not persisted: %+v", gotChild.Metadata)
	}
}

// TestSessionTouch confirms Touch bumps UpdatedAt + TurnCount and
// returns ErrNotFound for unknown ids.
func TestSessionTouch(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTempDB(t)

	created, _ := svc.Create(ctx, "touchy", "")

	if err := svc.Touch(ctx, created.ID); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	if err := svc.Touch(ctx, created.ID); err != nil {
		t.Fatalf("Touch second: %v", err)
	}

	got, _ := svc.Get(ctx, created.ID)
	if got.TurnCount != 2 {
		t.Fatalf("TurnCount = %d, want 2", got.TurnCount)
	}
	if !got.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %v, not after %v", got.UpdatedAt, created.UpdatedAt)
	}

	if err := svc.Touch(ctx, "nope"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Touch unknown = %v, want ErrNotFound", err)
	}
}

// TestSessionPersistAcrossReopen confirms data survives a DB close +
// reopen — durability is the whole point of moving off in-memory.
func TestSessionPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "lyra.db")

	db1, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	svc1 := sqlite.NewSessionService(db1)
	created, _ := svc1.Create(ctx, "persistent", "")
	_ = db1.Close()

	db2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer db2.Close()
	svc2 := sqlite.NewSessionService(db2)

	got, err := svc2.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Title != "persistent" {
		t.Fatalf("title = %q", got.Title)
	}
}

// TestMemoryUpsert confirms Update inserts on first write, overwrites
// on second, and List skips empty scopes.
func TestMemoryUpsert(t *testing.T) {
	ctx := context.Background()
	_, mem := newTempDB(t)

	// Get on empty DB returns "" not an error
	got, err := mem.Get(ctx, memory.ScopeProject)
	if err != nil {
		t.Fatalf("Get empty: %v", err)
	}
	if got != "" {
		t.Fatalf("Get empty = %q", got)
	}

	if err := mem.Update(ctx, memory.ScopeProject, "# project notes"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = mem.Get(ctx, memory.ScopeProject)
	if got != "# project notes" {
		t.Fatalf("Get after Update = %q", got)
	}

	// upsert
	if err := mem.Update(ctx, memory.ScopeProject, "# updated"); err != nil {
		t.Fatalf("Update 2: %v", err)
	}
	got, _ = mem.Get(ctx, memory.ScopeProject)
	if got != "# updated" {
		t.Fatalf("Get after upsert = %q", got)
	}

	// User scope independent
	if err := mem.Update(ctx, memory.ScopeUser, "# user notes"); err != nil {
		t.Fatalf("Update user: %v", err)
	}

	list, err := mem.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	// Ordered by scope (project=0 first)
	if list[0].Scope != memory.ScopeProject || list[0].Content != "# updated" {
		t.Fatalf("list[0] = %+v", list[0])
	}
	if list[1].Scope != memory.ScopeUser || list[1].Content != "# user notes" {
		t.Fatalf("list[1] = %+v", list[1])
	}
	if list[0].CapturedAt.IsZero() {
		t.Fatalf("CapturedAt not set")
	}
}

// TestHistoryStore_RoundTrip mirrors the file backend: items in append
// order (ORDER BY seq), RunRef upsert by run_id, per-session scoping.
func TestHistoryStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewHistoryStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, it := range []history.Item{
		{SessionID: "ses_a", RunID: "run_1", ItemID: "i1", CreatedAt: now, Blob: json.RawMessage(`{"id":"i1"}`)},
		{SessionID: "ses_a", RunID: "run_1", ItemID: "i2", CreatedAt: now, Blob: json.RawMessage(`{"id":"i2"}`)},
		{SessionID: "ses_b", RunID: "run_9", ItemID: "i9", CreatedAt: now, Blob: json.RawMessage(`{"id":"i9"}`)},
	} {
		if err := store.AppendItem(ctx, it); err != nil {
			t.Fatalf("append %s: %v", it.ItemID, err)
		}
	}
	if err := store.PutRun(ctx, history.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"running"}`)}); err != nil {
		t.Fatalf("put run running: %v", err)
	}
	if err := store.PutRun(ctx, history.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"finished"}`)}); err != nil {
		t.Fatalf("put run finished: %v", err)
	}

	items, runs, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ItemID != "i1" || items[1].ItemID != "i2" {
		t.Fatalf("items = %+v, want [i1 i2]", items)
	}
	if len(runs) != 1 || string(runs[0].Blob) != `{"status":"finished"}` {
		t.Fatalf("runs = %+v, want one finished run", runs)
	}
}
