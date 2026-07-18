package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/toolresultpreview"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func openTranscriptAndBlobs(t *testing.T) (*sqlite.TranscriptStore, *sqlite.ToolResultStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewTranscriptStore(db), sqlite.NewToolResultStore(db)
}

func toolItem(sessionID, id, result string, ref *resultoffload.Ref) transcript.Item {
	value := tool.StringResult(result)
	return transcript.Item{
		SessionID: sessionID,
		ID:        id,
		RunID:     "run-1",
		Kind:      transcript.ToolCall,
		Tool:      &transcript.ToolInvocation{Name: "shell", Result: &value, Offload: ref},
	}
}

func TestTranscriptRehydratesOffloadedToolResult(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-1"
	full := strings.Repeat("Z", 300)

	id := stageToolResult(t, blobs, sess, "shell", full)
	preview := toolresultpreview.Render(full, id, "read_tool_result", 100)
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

	items, _, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if got, ok := items[0].Tool.Result.String(); !ok || got != full {
		t.Fatalf("tool result not rehydrated: got %q, want the full %d-byte body", got, len(full))
	}
}

func TestTranscriptSurfacesMissingOffloadedToolResult(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-2"
	// A typed reference without its blob is durable corruption, not an ordinary
	// non-offloaded result, and must not be hidden as a harmless preview.
	preview := toolresultpreview.Render(strings.Repeat("q", 300), "GONE234BLOB", "read_tool_result", 100)
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", preview, &resultoffload.Ref{ID: "GONE234BLOB"})); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tr.List(t.Context(), sess); err == nil {
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
	items, _, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := items[0].Tool.Result.String(); got != plain {
		t.Fatalf("ordinary result altered: %q", got)
	}
}

func TestDeleteRunDropsItsBoundToolResults(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-drop"
	id := stageToolResult(t, blobs, sess, "shell", "full body")
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
