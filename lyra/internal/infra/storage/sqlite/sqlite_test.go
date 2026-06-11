package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/service/transcript"
)

func newTempDB(t *testing.T) *sqlite.SessionService {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewSessionService(db)
}

// TestSessionCRUD exercises the full mutate / read cycle of session.Service
// against the SQLite backend.
func TestSessionCRUD(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

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
	svc := newTempDB(t)

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
	_, err = svc.Fork(ctx, "nope", "msg-0")
	if !errors.Is(err, session.ErrNotFound) {
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
	svc := newTempDB(t)

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

// TestSessionRename confirms Rename updates the title + refreshes UpdatedAt
// and returns ErrNotFound for unknown ids.
func TestSessionRename(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	created, _ := svc.Create(ctx, "before", "")

	if err := svc.Rename(ctx, created.ID, "after"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, _ := svc.Get(ctx, created.ID)
	if got.Title != "after" {
		t.Fatalf("Title = %q, want after", got.Title)
	}
	if got.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %v, before %v", got.UpdatedAt, created.UpdatedAt)
	}

	if err := svc.Rename(ctx, "nope", "x"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Rename unknown = %v, want ErrNotFound", err)
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

// TestMessageStore_RoundTrip exercises the chat-memory store: append-order
// reads, per-conversation scoping, and Clear. Empty conversation reads as
// an empty slice; Clear is idempotent.
func TestMessageStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewMessageStore(db)
	ctx := context.Background()

	var got []chat.Message
	got, err = store.Read(ctx, "conv-a")
	if err != nil || len(got) != 0 {
		t.Fatalf("Read empty = %v (err %v), want empty", got, err)
	}

	err = store.Write(ctx, "conv-a", chat.NewUserMessage("hello"), chat.NewAssistantMessage("hi"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = store.Write(ctx, "conv-a", chat.NewUserMessage("again"))
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	err = store.Write(ctx, "conv-b", chat.NewUserMessage("other"))
	if err != nil {
		t.Fatalf("Write conv-b: %v", err)
	}

	got, err = store.Read(ctx, "conv-a")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("conv-a len = %d, want 3 (append order across writes)", len(got))
	}
	if u, ok := got[0].(*chat.UserMessage); !ok || u.Text != "hello" {
		t.Fatalf("got[0] = %#v, want user 'hello'", got[0])
	}
	if got2, _ := store.Read(ctx, "conv-b"); len(got2) != 1 {
		t.Fatalf("conv-b len = %d, want 1 (per-conversation scoping)", len(got2))
	}

	if err := store.Clear(ctx, "conv-a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got, _ := store.Read(ctx, "conv-a"); len(got) != 0 {
		t.Fatalf("after Clear conv-a len = %d, want 0", len(got))
	}
	if got2, _ := store.Read(ctx, "conv-b"); len(got2) != 1 {
		t.Fatalf("Clear leaked into conv-b: len = %d, want 1", len(got2))
	}
	if err := store.Clear(ctx, "conv-a"); err != nil {
		t.Fatalf("Clear idempotent: %v", err)
	}
}

// TestTranscriptStore_RoundTrip mirrors the file backend: items in append
// order (ORDER BY seq), RunRef upsert by run_id, per-session scoping.
func TestTranscriptStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, it := range []transcript.Item{
		{SessionID: "ses_a", RunID: "run_1", ItemID: "i1", CreatedAt: now, Blob: json.RawMessage(`{"id":"i1"}`)},
		{SessionID: "ses_a", RunID: "run_1", ItemID: "i2", CreatedAt: now, Blob: json.RawMessage(`{"id":"i2"}`)},
		{SessionID: "ses_b", RunID: "run_9", ItemID: "i9", CreatedAt: now, Blob: json.RawMessage(`{"id":"i9"}`)},
	} {
		err = store.AppendItem(ctx, it)
		if err != nil {
			t.Fatalf("append %s: %v", it.ItemID, err)
		}
	}
	err = store.PutRun(ctx, transcript.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"running"}`)})
	if err != nil {
		t.Fatalf("put run running: %v", err)
	}
	err = store.PutRun(ctx, transcript.Run{SessionID: "ses_a", RunID: "run_1", UpdatedAt: now, Blob: json.RawMessage(`{"status":"finished"}`)})
	if err != nil {
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

// TestSessionSubtaskLineage covers the delegation-lineage recording: a
// subtask child is stored under a caller-supplied id, inherits the parent's
// cwd, is marked KindSubtask, is hidden from List, yet is reachable via
// Children and Get. Idempotent on id.
func TestSessionSubtaskLineage(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	parent, err := svc.Create(ctx, "Parent", "/work/proj")
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	child, err := svc.CreateSubtask(ctx, "proc-123", parent.ID)
	if err != nil {
		t.Fatalf("CreateSubtask: %v", err)
	}
	if child.ID != "proc-123" {
		t.Errorf("child id = %q, want proc-123", child.ID)
	}
	if child.ParentID != parent.ID {
		t.Errorf("child ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Kind != session.KindSubtask {
		t.Errorf("child Kind = %q, want %q", child.Kind, session.KindSubtask)
	}
	if child.Cwd != "/work/proj" {
		t.Errorf("child Cwd = %q, want inherited /work/proj", child.Cwd)
	}

	// Idempotent: re-recording the same id returns the existing row.
	again, err := svc.CreateSubtask(ctx, "proc-123", parent.ID)
	if err != nil || again.ID != child.ID {
		t.Fatalf("CreateSubtask not idempotent: err=%v id=%q", err, again.ID)
	}

	// List hides subtask children — only the user-facing parent shows.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != parent.ID {
		t.Fatalf("List should show only the parent; got %d entries", len(list))
	}

	// Children surfaces the subtask under the parent (lineage queryable).
	kids, err := svc.Children(ctx, parent.ID)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(kids) != 1 || kids[0].ID != "proc-123" {
		t.Fatalf("Children(parent) = %+v, want one subtask proc-123", kids)
	}

	// Get resolves the subtask directly.
	got, err := svc.Get(ctx, "proc-123")
	if err != nil || got.ParentID != parent.ID {
		t.Fatalf("Get(subtask): err=%v parent=%q", err, got.ParentID)
	}
}
