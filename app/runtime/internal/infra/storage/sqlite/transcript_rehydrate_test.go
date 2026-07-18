package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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

func toolItem(sessionID, id, result string) transcript.Item {
	return transcript.Item{
		SessionID: sessionID,
		ID:        id,
		RunID:     "run-1",
		Kind:      transcript.ToolCall,
		Tool:      &transcript.ToolInvocation{Name: "shell", Result: result},
	}
}

func TestTranscriptRehydratesOffloadedToolResult(t *testing.T) {
	tr, blobs := openTranscriptAndBlobs(t)
	const sess = "sess-1"
	full := strings.Repeat("Z", 300)

	id, err := blobs.Offload(t.Context(), sess, "shell", full)
	if err != nil {
		t.Fatal(err)
	}
	placeholder := offload.Placeholder(full, id, "read_tool_result", 100)
	if len(placeholder) >= len(full) {
		t.Fatal("test setup: placeholder should be smaller than the full body")
	}
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", placeholder)); err != nil {
		t.Fatal(err)
	}

	items, _, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if got := items[0].Tool.Result; got != full {
		t.Fatalf("tool result not rehydrated: got %d bytes, want the full %d", len(got.(string)), len(full))
	}
}

func TestTranscriptLeavesPlaceholderWhenBlobMissing(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-2"
	// A placeholder whose blob was never written (or dropped): rehydration is
	// best-effort, so the item keeps the placeholder rather than erroring.
	placeholder := offload.Placeholder(strings.Repeat("q", 300), "GONE234BLOB", "read_tool_result", 100)
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", placeholder)); err != nil {
		t.Fatal(err)
	}
	items, _, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if got := items[0].Tool.Result; got != placeholder {
		t.Fatalf("missing blob should leave the placeholder, got %q", got)
	}
}

func TestTranscriptLeavesOrdinaryToolResultUntouched(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	const sess = "sess-3"
	const plain = "a normal, small tool result"
	if err := tr.AppendItem(t.Context(), toolItem(sess, "item-1", plain)); err != nil {
		t.Fatal(err)
	}
	items, _, err := tr.List(t.Context(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if got := items[0].Tool.Result; got != plain {
		t.Fatalf("ordinary result altered: %q", got)
	}
}
