package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/toolresultpreview"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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
	original := transcript.Item{
		SessionID: "ses_a", RunID: "run_1", ID: "item_1", CreatedAt: now,
		Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{
			Name: "shell", Result: "preview", Offload: &resultoffload.Ref{ID: "BLOB234"},
		},
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	changed := original
	changed.Tool = &transcript.ToolInvocation{
		Name: "shell", Result: "other preview", Offload: &resultoffload.Ref{ID: "OTHER234"},
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

// TestOpenDiscardsAnOlderSchema codifies the development contract: unknown
// obsolete shapes reset local state instead of growing compatibility readers.
func TestOpenDiscardsAnOlderSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacy.Exec(`CREATE TABLE legacy_runs (id TEXT PRIMARY KEY); INSERT INTO legacy_runs(id) VALUES ('old'); PRAGMA user_version = 1`); err != nil {
		_ = legacy.Close()
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open current schema: %v", err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version = %d, err=%v, want 7", version, err)
	}
	var legacyTables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='legacy_runs'`).Scan(&legacyTables); err != nil || legacyTables != 0 {
		t.Fatalf("legacy table count = %d, err=%v, want discarded", legacyTables, err)
	}
	var currentTables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&currentTables); err != nil || currentTables != 1 {
		t.Fatalf("sessions table count = %d, err=%v, want current schema", currentTables, err)
	}
}

// TestOpenMigratesV5AddsPortableToolResultsWithoutDataLoss is the regression
// for the v5→v7 additive migration: a database at version 5 must gain portable
// tool-result storage WITHOUT being discarded, so a user's sessions survive.
func TestOpenMigratesV5AddsPortableToolResultsWithoutDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sess, err := sqlite.NewSessionStore(db).Create(t.Context(), "keep me", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE tool_result_blobs`); err != nil {
		t.Fatalf("drop v6 table: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatalf("set version 5: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var version int
	if err := reopened.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version = %d, err=%v, want 7", version, err)
	}
	// The v5 data survived (not discarded)...
	var kept int
	if err := reopened.QueryRow(`SELECT count(*) FROM sessions WHERE id = ?`, sess.ID).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("session count = %d, err=%v, want the v5 session preserved", kept, err)
	}
	// ...and the new table is present and usable.
	if err := sqlite.NewToolResultStore(reopened).Stage(t.Context(), resultoffload.ToolResultStage{
		ID: resultoffload.NewID(), SessionID: sess.ID, ToolName: "shell", Body: "body",
	}); err != nil {
		t.Fatalf("tool_result_blobs unusable after migration: %v", err)
	}
}

func TestOpenMigratesV6BindsLegacyOffloadedResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open current: %v", err)
	}
	ses, err := sqlite.NewSessionStore(db).Create(t.Context(), "keep legacy offload", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	blobs := sqlite.NewToolResultStore(db)
	id := stageToolResult(t, blobs, ses.ID, "shell", "legacy full body")
	preview := toolresultpreview.Render("legacy full body", id, "read_tool_result", 8)
	item := transcript.Item{
		SessionID: ses.ID, RunID: "run_legacy", ID: "item_legacy", Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "shell", Result: preview},
	}
	if err := sqlite.NewTranscriptStore(db).AppendItem(t.Context(), item); err != nil {
		t.Fatalf("append legacy item: %v", err)
	}
	for _, statement := range []string{
		`DROP INDEX idx_tool_result_blobs_item`,
		`DROP INDEX idx_history_items_offload`,
		`ALTER TABLE history_items DROP COLUMN offload_id`,
		`ALTER TABLE tool_result_blobs DROP COLUMN item_id`,
		`ALTER TABLE tool_result_blobs DROP COLUMN preview`,
		`PRAGMA user_version = 6`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("downgrade fixture with %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v6 fixture: %v", err)
	}

	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("migrate v6: %v", err)
	}
	defer reopened.Close()
	items, _, err := sqlite.NewTranscriptStore(reopened).List(t.Context(), ses.ID)
	if err != nil {
		t.Fatalf("list migrated transcript: %v", err)
	}
	if len(items) != 1 || items[0].Tool == nil || items[0].Tool.Result != "legacy full body" {
		t.Fatalf("migrated items = %+v, want rehydrated legacy body", items)
	}
	ref := items[0].Tool.Offload
	if ref == nil || ref.ID != resultoffload.ID(id) {
		t.Fatalf("migrated ref = %+v, want %q", ref, id)
	}
}

func TestOpenMigratesV3ByDiscardingOnlyProcessContinuations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, cwd TEXT NOT NULL DEFAULT '', parent_id TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, metadata TEXT NOT NULL DEFAULT '{}',
			model TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL DEFAULT '', favorite INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO sessions(id,title,started_at,updated_at) VALUES ('ses_keep','kept',1,1);
		CREATE TABLE process_snapshots (id TEXT PRIMARY KEY, snapshot TEXT NOT NULL, captured_at INTEGER NOT NULL);
		INSERT INTO process_snapshots VALUES ('proc_old','{}',1);
		CREATE TABLE runs (
			run_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, state TEXT NOT NULL, provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', started_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
		);
		INSERT INTO runs(run_id,session_id,state,started_at,updated_at) VALUES ('run_old','ses_keep','interrupted',1,1);
		CREATE TABLE interrupts (
			run_id TEXT PRIMARY KEY, session_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
			process_id TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '', drained_tools TEXT NOT NULL DEFAULT '', run_created_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		);
		INSERT INTO interrupts(run_id,session_id,process_id,created_at) VALUES ('run_old','ses_keep','proc_old',1);
		PRAGMA user_version = 3;
	`)
	if err != nil {
		_ = legacy.Close()
		t.Fatalf("seed v3: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open v5: %v", err)
	}
	defer db.Close()
	var sessions, snapshots, interrupts int
	if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE id='ses_keep'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("preserved sessions = %d, err %v", sessions, err)
	}
	var userID, agentName string
	if err := db.QueryRow(`SELECT user_id, agent_name FROM sessions WHERE id='ses_keep'`).Scan(&userID, &agentName); err != nil || userID != "" || agentName != "" {
		t.Fatalf("migrated session identity = %q/%q, err %v", userID, agentName, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM process_snapshots`).Scan(&snapshots); err != nil || snapshots != 0 {
		t.Fatalf("snapshots = %d, err %v", snapshots, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM interrupts`).Scan(&interrupts); err != nil || interrupts != 0 {
		t.Fatalf("interrupts = %d, err %v", interrupts, err)
	}
	var state, outcome string
	if err := db.QueryRow(`SELECT state, outcome FROM runs WHERE run_id='run_old'`).Scan(&state, &outcome); err != nil || state != "terminal" || outcome != "snapshot_schema_incompatible" {
		t.Fatalf("migrated run = (%q,%q), err %v", state, outcome, err)
	}
}

func TestOpenMigratesV4SessionIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY, title TEXT NOT NULL, cwd TEXT NOT NULL DEFAULT '', parent_id TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, metadata TEXT NOT NULL DEFAULT '{}',
			model TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL DEFAULT '', favorite INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO sessions(id,title,started_at,updated_at) VALUES ('ses_keep','kept',1,1);
		CREATE TABLE process_snapshots (
			id TEXT PRIMARY KEY, revision INTEGER NOT NULL, snapshot TEXT NOT NULL, captured_at INTEGER NOT NULL
		);
		INSERT INTO process_snapshots VALUES ('proc_old',1,'{}',1);
		CREATE TABLE runs (
			run_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, state TEXT NOT NULL, provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', started_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
		);
		INSERT INTO runs(run_id,session_id,state,started_at,updated_at) VALUES ('run_old','ses_keep','running',1,1);
		CREATE TABLE interrupts (
			run_id TEXT PRIMARY KEY, session_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
			process_id TEXT NOT NULL DEFAULT '', provider TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '', drained_tools TEXT NOT NULL DEFAULT '', run_created_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		);
		INSERT INTO interrupts(run_id,session_id,process_id,created_at) VALUES ('run_old','ses_keep','proc_old',1);
		PRAGMA user_version = 4;
	`)
	if err != nil {
		_ = legacy.Close()
		t.Fatalf("seed v4: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open v5: %v", err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("schema version = %d, err=%v, want 7", version, err)
	}
	var userID, agentName string
	if err := db.QueryRow(`SELECT user_id, agent_name FROM sessions WHERE id='ses_keep'`).Scan(&userID, &agentName); err != nil || userID != "" || agentName != "" {
		t.Fatalf("migrated session identity = %q/%q, err %v", userID, agentName, err)
	}
	var snapshots, interrupts int
	if err := db.QueryRow(`SELECT count(*) FROM process_snapshots`).Scan(&snapshots); err != nil || snapshots != 0 {
		t.Fatalf("snapshots = %d, err %v", snapshots, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM interrupts`).Scan(&interrupts); err != nil || interrupts != 0 {
		t.Fatalf("interrupts = %d, err %v", interrupts, err)
	}
	var state, outcome string
	if err := db.QueryRow(`SELECT state, outcome FROM runs WHERE run_id='run_old'`).Scan(&state, &outcome); err != nil || state != "terminal" || outcome != "snapshot_schema_incompatible" {
		t.Fatalf("migrated run = (%q,%q), err %v", state, outcome, err)
	}
}

// TestSessionSubtaskLineage covers the delegation-lineage recording: a
// subtask child is stored under a caller-supplied id, inherits the parent's
// cwd, is marked KindSubtask, is hidden from List, yet is reachable via
// Children and Get. Re-saving may update audit metadata but not identity.
func TestSessionSubtaskLineage(t *testing.T) {
	ctx := context.Background()
	svc := newTempDB(t)

	parent, err := svc.Create(ctx, "Parent", "/work/proj")
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	now := time.Now().UTC()
	subtask := session.Subtask{
		ID:        "proc-123",
		ParentID:  parent.ID,
		UserID:    "user-1",
		AgentName: "research-agent",
		StartedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]any{"source": "runtime"},
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
	if child.UserID != subtask.UserID || child.AgentName != subtask.AgentName || child.Metadata["source"] != "runtime" {
		t.Errorf("child runtime identity = %#v, want %#v", child, subtask)
	}

	// Re-saving the same identity updates the durable runtime fields without
	// losing product-owned title/cwd enrichment.
	subtask.UpdatedAt = now.Add(time.Second)
	subtask.Metadata = map[string]any{"source": "updated"}
	again, err := svc.SaveSubtask(ctx, subtask)
	if err != nil || again.ID != child.ID ||
		again.Title != child.Title || again.Cwd != child.Cwd || again.Kind != child.Kind ||
		!again.UpdatedAt.Equal(subtask.UpdatedAt) || again.Metadata["source"] != "updated" {
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
