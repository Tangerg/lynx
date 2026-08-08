package conversationsearch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type stubSearch struct {
	gotQuery string
	gotLimit int
	hits     []transcript.SearchHit
}

func (s *stubSearch) SearchTranscript(_ context.Context, query string, limit int) ([]transcript.SearchHit, error) {
	s.gotQuery = query
	s.gotLimit = limit
	return s.hits, nil
}

func TestNewNilSearchOmitsTool(t *testing.T) {
	tool, err := New(nil)
	if err != nil || tool != nil {
		t.Fatalf("New(nil) = (%v, %v), want (nil, nil)", tool, err)
	}
}

func TestRunFormatsHits(t *testing.T) {
	when := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	stub := &stubSearch{hits: []transcript.SearchHit{
		{Kind: transcript.UserMessage, CreatedAt: when, Snippet: "how do we [deploy] the widget"},
		{Kind: transcript.AgentMessage, CreatedAt: when, Snippet: "the [deploy] runs via the pipeline"},
	}}
	tl, err := New(stub)
	if err != nil {
		t.Fatal(err)
	}
	if got := tl.Definition().Name; got != "search_conversations" {
		t.Fatalf("name = %q, want search_conversations", got)
	}
	text, err := tl.Call(t.Context(), `{"query":"  deploy widget  ","limit":20}`)
	if err != nil {
		t.Fatal(err)
	}
	// Query is trimmed and the accepted upper bound is forwarded unchanged.
	if stub.gotQuery != "deploy widget" {
		t.Fatalf("query = %q, want trimmed", stub.gotQuery)
	}
	if stub.gotLimit != 20 {
		t.Fatalf("limit = %d, want 20", stub.gotLimit)
	}
	for _, want := range []string{"1. [user · 2026-07-15]", "2. [agent · 2026-07-15]", "deploy"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestRunRejectsOutOfRangeLimit(t *testing.T) {
	tl, err := New(&stubSearch{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Call(t.Context(), `{"query":"deploy","limit":21}`); err == nil {
		t.Fatal("expected an error for limit above 20")
	}
}

func TestRunEmptyQueryErrors(t *testing.T) {
	tl, err := New(&stubSearch{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tl.Call(t.Context(), `{"query":"   "}`); err == nil {
		t.Fatal("expected an error for a blank query")
	}
}

func TestRunNoHits(t *testing.T) {
	tl, err := New(&stubSearch{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tl.Call(t.Context(), `{"query":"nothing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No earlier conversation matched") {
		t.Fatalf("unexpected empty-result text: %q", out)
	}
}
