package sqlite_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func msgItem(sessionID, id string, kind transcript.ItemKind, text string) transcript.Item {
	return transcript.Item{
		SessionID: sessionID,
		ID:        id,
		RunID:     "run-1",
		Kind:      kind,
		Content:   []transcript.ContentBlock{{Kind: transcript.TextContent, Text: text}},
	}
}

func TestTranscriptSearchIndexesConversationAndExcludesNoise(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(tr.AppendItem(ctx, msgItem("s1", "u1", transcript.UserMessage, "how do we deploy the widget service")))
	must(tr.AppendItem(ctx, msgItem("s1", "a1", transcript.AgentMessage, "the widget deploy runs through the release pipeline")))
	must(tr.AppendItem(ctx, msgItem("s2", "u2", transcript.UserMessage, "an unrelated conversation about lunch")))
	// A tool call is transcript noise — its content must not be searchable.
	must(tr.AppendItem(ctx, toolItem("s2", "t1", "widget widget widget", nil)))

	hits, err := tr.SearchTranscript(ctx, "widget deploy", 10)
	must(err)
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2 (the two s1 messages): %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.SessionID != "s1" {
			t.Fatalf("unexpected session %q in hit %+v", h.SessionID, h)
		}
		if !strings.Contains(strings.ToLower(h.Snippet), "widget") {
			t.Fatalf("snippet missing matched term: %q", h.Snippet)
		}
	}

	// Single-term search excludes the tool call and the unrelated session.
	widgetHits, err := tr.SearchTranscript(ctx, "widget", 10)
	must(err)
	if len(widgetHits) != 2 {
		t.Fatalf("widget hits = %d, want 2 (tool call + unrelated excluded): %+v", len(widgetHits), widgetHits)
	}
	for _, h := range widgetHits {
		if h.ItemID == "t1" {
			t.Fatal("a tool call was indexed for search")
		}
	}

	// A streamed message re-appends with its full text; the index must follow.
	must(tr.AppendItem(ctx, msgItem("s1", "a1", transcript.AgentMessage, "the widget deploy runs through the release pipeline and also kubernetes")))
	k, err := tr.SearchTranscript(ctx, "kubernetes", 10)
	must(err)
	if len(k) != 1 || k[0].ItemID != "a1" {
		t.Fatalf("kubernetes hits = %+v, want the updated a1", k)
	}

	// Empty / whitespace query is a no-op, not an error or a full scan.
	empty, err := tr.SearchTranscript(ctx, "   ", 10)
	must(err)
	if len(empty) != 0 {
		t.Fatalf("empty query returned %d hits", len(empty))
	}
}

func TestTranscriptSearchDeleteThrough(t *testing.T) {
	tr, _ := openTranscriptAndBlobs(t)
	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(tr.AppendItem(ctx, msgItem("s1", "u1", transcript.UserMessage, "topaz gemstone notes")))
	must(tr.AppendItem(ctx, msgItem("s2", "u2", transcript.UserMessage, "topaz appears here too")))

	if hits, err := tr.SearchTranscript(ctx, "topaz", 10); err != nil || len(hits) != 2 {
		t.Fatalf("before delete: hits=%d err=%v, want 2", len(hits), err)
	}

	// DeleteRun clears the run's search rows; DeleteSession clears the session's.
	must(tr.DeleteRun(ctx, "s1", "run-1"))
	if hits, err := tr.SearchTranscript(ctx, "topaz", 10); err != nil || len(hits) != 1 || hits[0].SessionID != "s2" {
		t.Fatalf("after DeleteRun: hits=%+v err=%v, want only s2", hits, err)
	}
	must(tr.DeleteSession(ctx, "s2"))
	if hits, err := tr.SearchTranscript(ctx, "topaz", 10); err != nil || len(hits) != 0 {
		t.Fatalf("after DeleteSession: hits=%d err=%v, want 0", len(hits), err)
	}
}
