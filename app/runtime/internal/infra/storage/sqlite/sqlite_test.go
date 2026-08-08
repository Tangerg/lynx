package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

func TestOpenHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "lyra.db")); !errors.Is(err, context.Canceled) {
		if db != nil {
			_ = db.Close()
		}
		t.Fatalf("Open error = %v, want context.Canceled", err)
	}
}

func newTempDB(t *testing.T) *sqlite.SessionStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
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

	// Child round-trips through Get as a pure product Session projection.
	if _, err := svc.Get(ctx, child.ID); err != nil {
		t.Fatalf("Get child: %v", err)
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

	db1, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	svc1 := sqlite.NewSessionStore(db1)
	created, _ := svc1.Create(ctx, "persistent", "")
	_ = db1.Close()

	db2, err := sqlite.Open(t.Context(), path)
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
	db, err := sqlite.Open(t.Context(), path)
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

// TestTranscriptStore_RoundTrip pins the item log: append order (ORDER BY seq)
// and per-session scoping. The Runs those items belong to are the run store's.
func TestTranscriptStore_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, it := range []transcript.Item{
		{SessionID: "ses_a", RunID: "run_1", ID: "i1", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.UserMessage, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "one"}}},
		{SessionID: "ses_a", RunID: "run_1", ID: "i2", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.AgentMessage, Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "two"}}},
		{SessionID: "ses_b", RunID: "run_9", ID: "i9", OccurredAt: now, Status: transcript.ItemCompleted, Kind: transcript.Reasoning, Text: "other"},
	} {
		err = store.AppendItem(ctx, it)
		if err != nil {
			t.Fatalf("append %s: %v", it.ID, err)
		}
	}
	items, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ID != "i1" || items[1].ID != "i2" || items[1].Content[0].Text != "two" {
		t.Fatalf("items = %+v, want [i1 i2]", items)
	}
}

func TestTranscriptStoreRejectsIdentityReparenting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	runs := sqlite.NewRunStore(db)
	ctx := t.Context()
	now := time.Now().UTC()

	if err := runs.Admit(ctx, run.RunDraft{SegmentID: "seg_open", RunID: "run_shared", SessionID: "ses_a", CreatedAt: now}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: "ses_a", RunID: "run_shared", ID: "item_shared", OccurredAt: now,
	}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	// A run id belongs to one session for its whole lifetime — and the refusal must
	// say so, not report the innocent session as busy.
	if err := runs.Admit(ctx, run.RunDraft{SegmentID: "seg_open", RunID: "run_shared", SessionID: "ses_b", CreatedAt: now}); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("re-parent run error = %v, want ErrIdentityConflict", err)
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: "ses_b", RunID: "run_other", ID: "item_shared", OccurredAt: now,
	}); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("re-parent item error = %v, want ErrIdentityConflict", err)
	}
	if err := store.AppendItem(ctx, transcript.Item{
		SessionID: "ses_a", RunID: "run_shared", ID: "item_shared", OccurredAt: now.Add(time.Second),
	}); !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("move item occurrence error = %v, want ErrIdentityConflict", err)
	}

	itemsA, err := store.List(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list ses_a: %v", err)
	}
	runsA, err := runs.ListRuns(ctx, "ses_a")
	if err != nil {
		t.Fatalf("list ses_a runs: %v", err)
	}
	itemsB, err := store.List(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list ses_b: %v", err)
	}
	runsB, err := runs.ListRuns(ctx, "ses_b")
	if err != nil {
		t.Fatalf("list ses_b runs: %v", err)
	}
	if len(itemsA) != 1 || itemsA[0].ID != "item_shared" || len(runsA) != 1 || runsA[0].ID != "run_shared" {
		t.Fatalf("original transcript changed: items=%+v runs=%+v", itemsA, runsA)
	}
	if len(itemsB) != 0 || len(runsB) != 0 {
		t.Fatalf("conflicting transcript was re-parented: items=%+v runs=%+v", itemsB, runsB)
	}
}

func TestTranscriptStoreReplaceItemUsesExactOptimisticSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	now := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	original := transcript.Item{
		SessionID:  "ses_a",
		RunID:      "run_1",
		ID:         "item_child",
		OccurredAt: now,
		FinishedAt: now,
		Status:     transcript.ItemIncomplete,
		Kind:       transcript.ToolCall,
		Tool:       &transcript.ToolInvocation{Name: "delegate_task", Arguments: tool.Arguments{}},
	}
	if err := store.AppendItem(t.Context(), original); err != nil {
		t.Fatalf("seed Item: %v", err)
	}
	problem := transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: "stop delegated branch",
	}
	replacement := original
	replacement.Error = &problem
	if err := store.ReplaceItem(t.Context(), original, replacement); err != nil {
		t.Fatalf("ReplaceItem: %v", err)
	}
	stored, found, err := store.Item(t.Context(), original.ID)
	if err != nil || !found {
		t.Fatalf("Item after replacement found=%t err=%v", found, err)
	}
	if stored.Error == nil || stored.Error.Kind != transcript.ChildRunCanceledProblem {
		t.Fatalf("replaced Item = %+v, want child_run_canceled", stored)
	}

	staleReplacement := replacement
	staleReplacement.Error = &transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: "overwrite newer result",
	}
	err = store.ReplaceItem(t.Context(), original, staleReplacement)
	if !errors.Is(err, transcript.ErrIdentityConflict) {
		t.Fatalf("stale ReplaceItem error = %v, want ErrIdentityConflict", err)
	}
	stored, found, err = store.Item(t.Context(), original.ID)
	if err != nil || !found || stored.Error == nil || stored.Error.Detail != problem.Detail {
		t.Fatalf("Item after stale replacement = %+v found=%t err=%v", stored, found, err)
	}
}

func TestTranscriptStoreKeepsOffloadRelationshipsImmutableAndOneToOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewTranscriptStore(db)
	now := time.Now().UTC()
	preview := tool.StringResult("preview")
	original := transcript.Item{
		SessionID: "ses_a", RunID: "run_1", ID: "item_1", OccurredAt: now,
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

// TestOpenRefusesEveryMismatchedSchemaWithoutTouchingIt pins the pre-release
// storage contract: only the current shape is supported, including for an
// unstamped non-empty database, and no old epoch receives a compatibility path.
// Refusing is the whole point — the file holds the user's sessions and
// credentials, so deciding to throw them away is theirs, not the runtime's.
func TestOpenRefusesEveryMismatchedSchemaWithoutTouchingIt(t *testing.T) {
	for _, staleEpoch := range []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 25} {
		t.Run(fmt.Sprintf("epoch_%d", staleEpoch), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stale.db")
			stale, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("open stale database: %v", err)
			}
			_, seedErr := stale.Exec(fmt.Sprintf(
				`CREATE TABLE stale_runs (id TEXT PRIMARY KEY); INSERT INTO stale_runs(id) VALUES ('old'); PRAGMA user_version = %d`,
				staleEpoch,
			))
			if seedErr != nil {
				_ = stale.Close()
				t.Fatalf("seed stale schema: %v", seedErr)
			}
			if err := stale.Close(); err != nil {
				t.Fatalf("close stale database: %v", err)
			}

			db, err := sqlite.Open(t.Context(), path)
			if !errors.Is(err, sqlite.ErrSchemaEpochMismatch) {
				if err == nil {
					_ = db.Close()
				}
				t.Fatalf("open stale epoch %d error = %v, want ErrSchemaEpochMismatch", staleEpoch, err)
			}
			// The refusal names the file, because "delete it to rebuild" is only
			// actionable if the user knows which file.
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("refusal %q does not name %q", err, path)
			}

			// Nothing was dropped and no schema was installed alongside the old one.
			reopened, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("reopen refused database: %v", err)
			}
			defer reopened.Close()
			var rows int
			if err := reopened.QueryRow(`SELECT count(*) FROM stale_runs`).Scan(&rows); err != nil || rows != 1 {
				t.Fatalf("stale rows = %d, err=%v, want the untouched seed", rows, err)
			}
			var installed int
			if err := reopened.QueryRow(
				`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='sessions'`,
			).Scan(&installed); err != nil || installed != 0 {
				t.Fatalf("current-schema tables = %d, err=%v, want none installed", installed, err)
			}
		})
	}
}

// A database this process just created is not a mismatch: an empty file carries
// no durable state to lose, so it is installed into rather than refused. Without
// this exception every first run would fail.
func TestOpenInstallsIntoAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	empty, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("create empty database: %v", err)
	}
	if _, err := empty.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("touch empty database: %v", err)
	}
	if err := empty.Close(); err != nil {
		t.Fatalf("close empty database: %v", err)
	}

	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open empty database: %v", err)
	}
	defer db.Close()
	var epoch int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&epoch); err != nil || epoch == 0 {
		t.Fatalf("installed epoch = %d, err=%v, want the current epoch", epoch, err)
	}
}
