package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

func newTempDB(t *testing.T) *sqlite.SessionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewSessionStore(db)
}

// TestSessionCRUD exercises the full mutate / read cycle of session.Store
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

// TestSessionFork confirms a child session is linked to its parent without
// inheriting unrelated parent state.
func TestSessionFork(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	parent, _ := svc.Create(ctx, "parent", "")

	child, err := svc.Fork(ctx, parent.ID)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("child.ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Title != "parent (fork)" {
		t.Fatalf("child title = %q", child.Title)
	}

	// fork of unknown parent → ErrNotFound
	_, err = svc.Fork(ctx, "nope")
	if !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Fork unknown parent = %v, want ErrNotFound", err)
	}

	// child round-trips through Get with canonical empty agent annotations.
	gotChild, err := svc.Get(ctx, child.ID)
	if err != nil {
		t.Fatalf("Get child: %v", err)
	}
	if gotChild.AgentAnnotations.String() != "{}" {
		t.Fatalf("agent annotations round trip = %s, want {}", gotChild.AgentAnnotations.String())
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

// TestSessionRenameIfUntitled confirms the auto-titler's atomic write only
// lands on a still-untitled session and is a no-op (nil) otherwise — the
// clobber protection against a concurrent user rename.
func TestSessionRenameIfUntitled(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	// Untitled → sets.
	untitled, _ := svc.Create(ctx, "", "")
	if err := svc.RenameIfUntitled(ctx, untitled.ID, "auto"); err != nil {
		t.Fatalf("RenameIfUntitled: %v", err)
	}
	if got, _ := svc.Get(ctx, untitled.ID); got.Title != "auto" {
		t.Fatalf("Title = %q, want auto", got.Title)
	}

	// Already titled (the user renamed during generation) → no-op, keeps the
	// user's title, no error.
	titled, _ := svc.Create(ctx, "mine", "")
	if err := svc.RenameIfUntitled(ctx, titled.ID, "auto"); err != nil {
		t.Fatalf("RenameIfUntitled titled = %v, want nil", err)
	}
	if got, _ := svc.Get(ctx, titled.ID); got.Title != "mine" {
		t.Fatalf("Title = %q, want the user's title preserved", got.Title)
	}

	// Unknown id → no-op nil (best-effort, not ErrNotFound).
	if err := svc.RenameIfUntitled(ctx, "nope", "x"); err != nil {
		t.Fatalf("RenameIfUntitled unknown = %v, want nil", err)
	}
}

func TestSessionFavorite(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	created, _ := svc.Create(ctx, "s", "")
	if created.Favorite {
		t.Fatal("new session must not be favorited")
	}

	if err := svc.SetFavorite(ctx, created.ID, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	if got, _ := svc.Get(ctx, created.ID); !got.Favorite {
		t.Fatal("Favorite = false after SetFavorite(true)")
	}

	if err := svc.SetFavorite(ctx, created.ID, false); err != nil {
		t.Fatalf("SetFavorite(false): %v", err)
	}
	if got, _ := svc.Get(ctx, created.ID); got.Favorite {
		t.Fatal("Favorite = true after SetFavorite(false)")
	}

	if err := svc.SetFavorite(ctx, "nope", true); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("SetFavorite unknown = %v, want ErrNotFound", err)
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
	svc1 := sqlite.NewSessionStore(db1)
	created, _ := svc1.Create(ctx, "persistent", "")
	_ = db1.Close()

	db2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer db2.Close()
	svc2 := sqlite.NewSessionStore(db2)

	got, err := svc2.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Title != "persistent" {
		t.Fatalf("title = %q", got.Title)
	}
}

// TestMessageStore_RoundTrip exercises the conversation message store: append-order
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

	err = store.Write(ctx, "conv-a", chat.NewUserMessage(chat.NewTextPart("hello")), chat.NewAssistantMessage(chat.NewTextPart("hi")))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = store.Write(ctx, "conv-a", chat.NewUserMessage(chat.NewTextPart("again")))
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	err = store.Write(ctx, "conv-b", chat.NewUserMessage(chat.NewTextPart("other")))
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
	if got[0].Role != chat.RoleUser || got[0].Text() != "hello" {
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
		{SessionID: "ses_a", RunID: "run_1", ID: "i1", CreatedAt: now, Status: transcript.ItemCompleted, Kind: transcript.UserMessage, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "one"}}},
		{SessionID: "ses_a", RunID: "run_1", ID: "i2", CreatedAt: now, Status: transcript.ItemCompleted, Kind: transcript.AgentMessage, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "two"}}},
		{SessionID: "ses_b", RunID: "run_9", ID: "i9", CreatedAt: now, Status: transcript.ItemCompleted, Kind: transcript.Reasoning, Text: "other"},
	} {
		err = store.AppendItem(ctx, it)
		if err != nil {
			t.Fatalf("append %s: %v", it.ID, err)
		}
	}
	err = store.PutRun(ctx, transcript.Run{SessionID: "ses_a", ID: "run_1", State: execution.Running, UpdatedAt: now, MessageMark: -1})
	if err != nil {
		t.Fatalf("put run running: %v", err)
	}
	outcome := execution.OutcomeCompleted
	err = store.PutRun(ctx, transcript.Run{SessionID: "ses_a", ID: "run_1", State: execution.Completed, Outcome: &outcome, UpdatedAt: now, MessageMark: 3})
	if err != nil {
		t.Fatalf("put run finished: %v", err)
	}

	items, runs, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ID != "i1" || items[1].ID != "i2" || items[1].Content[0].Text != "two" {
		t.Fatalf("items = %+v, want [i1 i2]", items)
	}
	if len(runs) != 1 || runs[0].State != execution.Completed || runs[0].Outcome == nil || *runs[0].Outcome != execution.OutcomeCompleted || runs[0].MessageMark != 3 {
		t.Fatalf("runs = %+v, want one finished run", runs)
	}
}

func TestTranscriptStoreRejectsIdentityReparenting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	ctx := t.Context()
	now := time.Now().UTC()

	if err := store.PutRun(ctx, transcript.Run{SessionID: "ses_a", ID: "run_shared", UpdatedAt: now}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: "ses_a", RunID: "run_shared", ID: "item_shared", CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	if err := store.PutRun(ctx, transcript.Run{SessionID: "ses_b", ID: "run_shared", UpdatedAt: now}); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("re-parent run error = %v, want ErrIdentityConflict", err)
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: "ses_b", RunID: "run_other", ID: "item_shared", CreatedAt: now,
	}); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("re-parent item error = %v, want ErrIdentityConflict", err)
	}

	itemsA, runsA, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list ses_a: %v", err)
	}
	itemsB, runsB, err := store.List(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list ses_b: %v", err)
	}
	if len(itemsA) != 1 || itemsA[0].ID != "item_shared" || len(runsA) != 1 || runsA[0].ID != "run_shared" {
		t.Fatalf("original transcript changed: items=%+v runs=%+v", itemsA, runsA)
	}
	if len(itemsB) != 0 || len(runsB) != 0 {
		t.Fatalf("conflicting transcript was re-parented: items=%+v runs=%+v", itemsB, runsB)
	}
}

func TestTranscriptStoreKeepsOffloadRelationshipsImmutableAndOneToOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	now := time.Now().UTC()
	preview := tool.StringResult("preview")
	original := transcript.Item{
		SessionID: "ses_a", RunID: "run_1", ID: "item_1", CreatedAt: now,
		Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{
			Name: "shell", Result: &preview, Offload: &resultoffload.Ref{ID: "BLOB234"},
		},
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	changed := original
	otherPreview := tool.StringResult("other preview")
	changed.Tool = &transcript.ToolInvocation{
		Name: "shell", Result: &otherPreview, Offload: &resultoffload.Ref{ID: "OTHER234"},
	}
	if err := store.AppendItem(t.Context(), changed); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("replace offload error = %v, want ErrIdentityConflict", err)
	}

	duplicate := original
	duplicate.ID = "item_2"
	if err := store.AppendItem(t.Context(), duplicate); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("reuse offload error = %v, want ErrIdentityConflict", err)
	}
}

// TestOpenDiscardsEveryMismatchedSchema pins the pre-release storage contract:
// only the current shape is supported, including for an unversioned non-empty
// database. No old version receives a compatibility path.
func TestOpenDiscardsEveryMismatchedSchema(t *testing.T) {
	for _, staleVersion := range []int{0, 1, 3, 4, 5, 6, 7, 8, 9} {
		t.Run(fmt.Sprintf("version_%d", staleVersion), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stale.db")
			stale, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("open stale database: %v", err)
			}
			_, seedErr := stale.Exec(fmt.Sprintf(
				`CREATE TABLE stale_runs (id TEXT PRIMARY KEY); INSERT INTO stale_runs(id) VALUES ('old'); PRAGMA user_version = %d`,
				staleVersion,
			))
			if seedErr != nil {
				_ = stale.Close()
				t.Fatalf("seed stale schema: %v", seedErr)
			}
			if err := stale.Close(); err != nil {
				t.Fatalf("close stale database: %v", err)
			}

			db, err := sqlite.Open(path)
			if err != nil {
				t.Fatalf("open current schema: %v", err)
			}
			defer db.Close()

			var version int
			if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 15 {
				t.Fatalf("schema version = %d, err=%v, want 15", version, err)
			}
			var staleTables int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='stale_runs'`).Scan(&staleTables); err != nil || staleTables != 0 {
				t.Fatalf("stale table count = %d, err=%v, want discarded", staleTables, err)
			}
			var currentTables int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&currentTables); err != nil || currentTables != 1 {
				t.Fatalf("sessions table count = %d, err=%v, want current schema", currentTables, err)
			}
		})
	}
}

// TestSessionSubtaskLineage covers the delegation-lineage recording: a
// subtask child is stored under a caller-supplied id, inherits the parent's
// cwd, is marked KindSubtask, is hidden from List, yet is reachable via
// Children and Get. Re-saving may update agent annotations but not identity.
func TestSessionSubtaskLineage(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	parent, err := svc.Create(ctx, "Parent", "/work/proj")
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	now := time.Now().UTC()
	annotations, err := session.ParseAgentAnnotations([]byte(`{"source":"runtime"}`))
	if err != nil {
		t.Fatalf("ParseAgentAnnotations: %v", err)
	}
	subtask := session.Subtask{
		ID:               "proc-123",
		ParentID:         parent.ID,
		UserID:           "user-1",
		AgentName:        "research-agent",
		StartedAt:        now,
		UpdatedAt:        now,
		AgentAnnotations: annotations,
	}
	child, err := svc.SaveSubtask(ctx, subtask)
	if err != nil {
		t.Fatalf("SaveSubtask: %v", err)
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
	if child.UserID != subtask.UserID || child.AgentName != subtask.AgentName || child.AgentAnnotations.String() != `{"source":"runtime"}` {
		t.Errorf("child runtime identity = %#v, want %#v", child, subtask)
	}

	// Re-saving the same identity updates the durable runtime fields without
	// losing product-owned title/cwd enrichment.
	subtask.UpdatedAt = now.Add(time.Second)
	subtask.AgentAnnotations, err = session.ParseAgentAnnotations([]byte(`{"source":"updated"}`))
	if err != nil {
		t.Fatalf("ParseAgentAnnotations update: %v", err)
	}
	again, err := svc.SaveSubtask(ctx, subtask)
	if err != nil || again.ID != child.ID ||
		again.Title != child.Title || again.Cwd != child.Cwd || again.Kind != child.Kind ||
		!again.UpdatedAt.Equal(subtask.UpdatedAt) || again.AgentAnnotations.String() != `{"source":"updated"}` {
		t.Fatalf("SaveSubtask update = (%#v, %v)", again, err)
	}
	for name, mutate := range map[string]func(*session.Subtask){
		"parent": func(s *session.Subtask) { s.ParentID = "other-parent" },
		"user":   func(s *session.Subtask) { s.UserID = "other-user" },
		"agent":  func(s *session.Subtask) { s.AgentName = "other-agent" },
		"start":  func(s *session.Subtask) { s.StartedAt = s.StartedAt.Add(time.Second) },
	} {
		t.Run("conflicting "+name, func(t *testing.T) {
			conflict := subtask
			mutate(&conflict)
			if _, err := svc.SaveSubtask(ctx, conflict); !errors.Is(err, session.ErrSubtaskConflict) {
				t.Fatalf("SaveSubtask conflict = %v, want ErrSubtaskConflict", err)
			}
		})
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
