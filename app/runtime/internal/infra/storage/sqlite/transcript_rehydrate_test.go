package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
)

func openTranscriptAndBlobs(t *testing.T) (*sqlite.TranscriptStore, *sqlite.ToolResultStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewTranscriptStore(db), sqlite.NewToolResultStore(db)
}

func toolItem(sessionID, id, result string, ref *resultoffload.Ref) transcript.Item {
	value := tool.StringResult(result)
	at := time.Unix(1, 0).UTC()
	return itemfixture.MustRestore(itemfixture.Input{
		SessionID:  sessionID,
		ID:         id,
		RunID:      "run-1",
		Kind:       transcript.ToolCall,
		Status:     transcript.ItemCompleted,
		OccurredAt: at,
		FinishedAt: at,
		Tool:       &transcript.ToolInvocation{Name: "shell", Result: &value, Offload: ref},
	})
}

func TestTranscriptPaginationRejectsInvalidControls(t *testing.T) {
	store, _ := openTranscriptAndBlobs(t)

	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.SequenceOrder("ascending"), 0, 1); err == nil {
		t.Fatal("PageSessionItems accepted an unknown order")
	}
	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.OldestFirst, -1, 1); err == nil {
		t.Fatal("PageSessionItems accepted a negative sequence")
	}
	if _, err := store.PageSessionItems(t.Context(), "sess-1", transcript.OldestFirst, 0, -1); err == nil {
		t.Fatal("PageSessionItems accepted a negative limit")
	}
}

func TestTranscriptRehydratesOffloadedToolResult(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-1"
	full := strings.Repeat("Z", 300)

	id := stageShellResult(t, blobs, sess, full)
	preview := "offloaded preview"
	if len(preview) >= len(full) {
		t.Fatal("test setup: preview should be smaller than the full body")
	}
	ref := &resultoffload.Ref{ID: id}
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", preview, ref)); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Bind(t.Context(), sess, "item-1", preview, *ref); err != nil {
		t.Fatal(err)
	}

	items, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	invocation, present := items[0].ToolInvocation()
	if !present {
		t.Fatal("rehydrated Item has no Tool invocation")
	}
	if got, ok := invocation.Result.String(); !ok || got != full {
		t.Fatalf("tool result not rehydrated: got %q, want the full %d-byte body", got, len(full))
	}
}

func TestTranscriptSurfacesMissingOffloadedToolResult(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-2"
	// A typed reference without its blob is durable corruption, not an ordinary
	// non-offloaded result, and must not be hidden as a harmless preview.
	preview := "missing offloaded preview"
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", preview, &resultoffload.Ref{ID: "GONE234BLOB"})); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.List(t.Context(), sess); err == nil {
		t.Fatal("missing blob must surface a broken durable reference")
	}
}

func TestTranscriptLeavesOrdinaryToolResultUntouched(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-3"
	const plain = "a normal, small tool result"
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", plain, nil)); err != nil {
		t.Fatal(err)
	}
	items, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	invocation, present := items[0].ToolInvocation()
	if !present {
		t.Fatal("stored Item has no Tool invocation")
	}
	if got, _ := invocation.Result.String(); got != plain {
		t.Fatalf("ordinary result altered: %q", got)
	}
}

func TestDeleteRunDropsItsBoundToolResults(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-drop"
	id := stageShellResult(t, blobs, sess, "full body")
	ref := &resultoffload.Ref{ID: id}
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", "preview", ref)); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Bind(t.Context(), sess, "item-1", "preview", *ref); err != nil {
		t.Fatal(err)
	}
	if err := tr.DeleteRun(t.Context(), sess, "run-1"); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	if _, found, err := blobs.Fetch(t.Context(), sess, id); err != nil || found {
		t.Fatalf("blob after DeleteRun = (found %v, err %v), want removed", found, err)
	}
}
