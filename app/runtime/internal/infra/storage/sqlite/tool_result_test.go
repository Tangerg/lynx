package sqlite_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
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

func stageToolResult(t *testing.T, store *sqlite.ToolResultStore, sessionID, toolName, body string) offload.ID {
	t.Helper()
	id := offload.NewID()
	if err := store.Stage(t.Context(), offload.ToolResultStage{
		ID: id, SessionID: sessionID, ToolName: toolName, Body: body,
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	return id
}

func TestToolResultStageRoundTrip(t *testing.T) {
	store := newToolResultStore(t)
	const (
		sessID = "sess-1"
		body   = "the full, oversized tool output that was offloaded"
	)
	id := stageToolResult(t, store, sessID, "shell", body)

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

func TestToolResultFetchIsSessionScoped(t *testing.T) {
	store := newToolResultStore(t)
	id := stageToolResult(t, store, "owner", "shell", "secret")
	// A different session must not read another session's offloaded body.
	if _, found, err := store.Fetch(t.Context(), "intruder", id); err != nil || found {
		t.Fatalf("cross-session fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultFetchUnknownIDIsRecoverableMiss(t *testing.T) {
	store := newToolResultStore(t)
	if _, found, err := store.Fetch(t.Context(), "s", "DOESNOTEXIST"); err != nil || found {
		t.Fatalf("unknown id = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultDropSession(t *testing.T) {
	store := newToolResultStore(t)
	id := stageToolResult(t, store, "doomed", "shell", "body")
	if err := store.DropSession(t.Context(), "doomed"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "doomed", id); err != nil || found {
		t.Fatalf("post-drop fetch = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestToolResultDiscardAndStartupPurgeOnlyRemoveUnboundBlobs(t *testing.T) {
	store := newToolResultStore(t)
	discardedID := stageToolResult(t, store, "ses_1", "shell", "discard me")
	if err := store.Discard(t.Context(), "ses_1", offload.Ref{ID: discardedID}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "ses_1", discardedID); err != nil || found {
		t.Fatalf("discarded fetch = (found %v, err %v), want (false, nil)", found, err)
	}

	boundID := stageToolResult(t, store, "ses_1", "shell", "keep me")
	boundRef := offload.Ref{ID: boundID}
	if err := store.Bind(t.Context(), "ses_1", "item_1", "preview", boundRef); err != nil {
		t.Fatal(err)
	}
	if err := store.Discard(t.Context(), "ses_1", boundRef); err != nil {
		t.Fatalf("discard bound: %v", err)
	}
	if _, found, err := store.Fetch(t.Context(), "ses_1", boundID); err != nil || !found {
		t.Fatalf("bound fetch after discard = (found %v, err %v), want (true, nil)", found, err)
	}

	stageToolResult(t, store, "ses_1", "shell", "stale after crash")
	removed, err := store.PurgeUnbound(t.Context())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if removed != 1 {
		t.Fatalf("purged = %d, want 1", removed)
	}
	if body, found, err := store.Fetch(t.Context(), "ses_1", boundID); err != nil || !found || body != "keep me" {
		t.Fatalf("bound fetch after purge = (%q, %v, %v)", body, found, err)
	}
}

func TestToolResultStoreRejectsIncompleteIdentity(t *testing.T) {
	store := newToolResultStore(t)
	valid := offload.ToolResultStage{ID: offload.NewID(), SessionID: "ses_1", ToolName: "shell", Body: "body"}
	missingSession := valid
	missingSession.SessionID = ""
	if err := store.Stage(t.Context(), missingSession); err == nil {
		t.Fatal("Stage accepted an empty session ID")
	}
	missingTool := valid
	missingTool.ToolName = ""
	if err := store.Stage(t.Context(), missingTool); err == nil {
		t.Fatal("Stage accepted an empty tool name")
	}
	missingBody := valid
	missingBody.Body = ""
	if err := store.Stage(t.Context(), missingBody); err == nil {
		t.Fatal("Stage accepted an empty body")
	}
}

func TestToolResultBindingListAndRestore(t *testing.T) {
	store := newToolResultStore(t)
	id := stageToolResult(t, store, "source", "shell", "full body")
	ref := offload.Ref{ID: id}
	if err := store.Bind(t.Context(), "source", "item_1", "preview", ref); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.Bind(t.Context(), "source", "item_1", "preview", ref); err != nil {
		t.Fatalf("replayed bind: %v", err)
	}
	if err := store.Bind(t.Context(), "source", "item_2", "other", ref); !errors.Is(err, offload.ErrIdentityConflict) {
		t.Fatalf("conflicting bind = %v, want ErrIdentityConflict", err)
	}

	blobs, err := store.List(t.Context(), "source")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(blobs) != 1 || blobs[0].ID != id || blobs[0].ItemID != "item_1" || blobs[0].Preview != "preview" || blobs[0].Body != "full body" {
		t.Fatalf("listed blobs = %+v, want exact bound blob", blobs)
	}
	blob := blobs[0]
	if err := store.DropSession(t.Context(), "source"); err != nil {
		t.Fatal(err)
	}
	blob.SessionID = "restored"
	blob.CreatedAt = time.Unix(blob.CreatedAt.Unix(), 0).UTC()
	if err := store.Restore(t.Context(), blob); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got, found, err := store.Fetch(t.Context(), "restored", id); err != nil || !found || got != "full body" {
		t.Fatalf("restored fetch = (%q, %v, %v)", got, found, err)
	}
}

func TestToolResultRestoreNeverReparentsAnID(t *testing.T) {
	store := newToolResultStore(t)
	id := stageToolResult(t, store, "owner", "shell", "body")
	blob := offload.ToolResultBlob{
		ID: id, SessionID: "intruder", ItemID: "item_1", ToolName: "shell",
		Preview: "preview", Body: "body", CreatedAt: time.Now().UTC(),
	}
	if err := store.Restore(t.Context(), blob); !errors.Is(err, offload.ErrIdentityConflict) {
		t.Fatalf("Restore() error = %v, want ErrIdentityConflict", err)
	}
}
