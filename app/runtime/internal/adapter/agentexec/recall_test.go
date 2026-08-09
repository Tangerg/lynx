package agentexec

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

type fakeAgentMemorySearcher struct {
	items []agentmemory.Item
	err   error
	query string
}

func (f *fakeAgentMemorySearcher) Search(_ context.Context, _ agentmemory.Scope, _, query string, _ int) ([]agentmemory.Item, error) {
	f.query = query
	return f.items, f.err
}

func TestRecalledMemoriesSkipsPinnedAndInjectsRest(t *testing.T) {
	search := &fakeAgentMemorySearcher{items: []agentmemory.Item{
		{Content: "- pinned core", Pinned: true},
		{Content: "- relevant fact", Pinned: false},
	}}
	msg, ok := recalledMemories(context.Background(), search, "/repo", "what is the fact")
	if !ok {
		t.Fatal("expected a recall block")
	}
	text := msg.Text()
	if strings.Contains(text, "pinned core") {
		t.Fatalf("pinned item must not appear in the recall block:\n%s", text)
	}
	if !strings.Contains(text, "relevant fact") {
		t.Fatalf("relevant fact missing:\n%s", text)
	}
	if search.query != "what is the fact" {
		t.Fatalf("query passed to searcher = %q", search.query)
	}
}

func TestRecalledMemoriesEmptyCases(t *testing.T) {
	if _, ok := recalledMemories(context.Background(), nil, "/repo", "q"); ok {
		t.Fatal("no searcher → no block")
	}
	if _, ok := recalledMemories(context.Background(), &fakeAgentMemorySearcher{}, "/repo", "q"); ok {
		t.Fatal("no items → no block")
	}
	allPinned := &fakeAgentMemorySearcher{items: []agentmemory.Item{{Content: "- x", Pinned: true}}}
	if _, ok := recalledMemories(context.Background(), allPinned, "/repo", "q"); ok {
		t.Fatal("all-pinned results → no block (already in the core)")
	}
	if _, ok := recalledMemories(context.Background(), &fakeAgentMemorySearcher{}, "/repo", "  "); ok {
		t.Fatal("blank query → no block")
	}
}
