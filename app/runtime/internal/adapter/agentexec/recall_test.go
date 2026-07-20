package agentexec

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

type fakeMemorySearcher struct {
	items []agentmemory.Item
	err   error
	query string
}

func (f *fakeMemorySearcher) Search(_ context.Context, _ agentmemory.Scope, _, query string, _ int) ([]agentmemory.Item, error) {
	f.query = query
	return f.items, f.err
}

func TestRecalledMemoriesSkipsPinnedAndInjectsRest(t *testing.T) {
	search := &fakeMemorySearcher{items: []agentmemory.Item{
		{Content: "- pinned core", Pinned: true},
		{Content: "- relevant fact", Pinned: false},
	}}
	e := &Engine{memorySearch: search, workdir: "/repo"}

	msg, ok := e.recalledMemories(context.Background(), "what is the fact")
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
	if _, ok := (&Engine{}).recalledMemories(context.Background(), "q"); ok {
		t.Fatal("no searcher → no block")
	}
	if _, ok := (&Engine{memorySearch: &fakeMemorySearcher{}, workdir: "/repo"}).recalledMemories(context.Background(), "q"); ok {
		t.Fatal("no items → no block")
	}
	allPinned := &Engine{memorySearch: &fakeMemorySearcher{items: []agentmemory.Item{{Content: "- x", Pinned: true}}}, workdir: "/repo"}
	if _, ok := allPinned.recalledMemories(context.Background(), "q"); ok {
		t.Fatal("all-pinned results → no block (already in the core)")
	}
	if _, ok := (&Engine{memorySearch: &fakeMemorySearcher{}, workdir: "/repo"}).recalledMemories(context.Background(), "  "); ok {
		t.Fatal("blank query → no block")
	}
}
