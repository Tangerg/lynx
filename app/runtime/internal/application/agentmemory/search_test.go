package agentmemory

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

type fakeItemSource struct {
	items []domain.Item
	err   error
}

func (f fakeItemSource) ItemsForSearch(context.Context, domain.Scope, string) ([]domain.Item, error) {
	return f.items, f.err
}

type fakeEmbedder struct {
	id      string
	vectors map[string][]float32
	err     error
}

func (f fakeEmbedder) ID() string {
	if f.id != "" {
		return f.id
	}
	return "fake"
}

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = f.vectors[text]
	}
	return out, nil
}

func items(specs ...domain.Item) []domain.Item { return specs }

func TestSearchKeywordOnlyWhenNoEmbedder(t *testing.T) {
	store := fakeItemSource{items: items(
		domain.Item{ID: "a", Content: "- run make test to build"},
		domain.Item{ID: "b", Content: "- prefer tabs over spaces"},
		domain.Item{ID: "c", Content: "- deploy with kubectl apply"},
	)}
	s := NewSearcher(store, nil)
	got, err := s.Search(context.Background(), domain.ScopeProject, "/repo", "how do we run tests", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("keyword search = %+v, want just item a", got)
	}
}

func TestSearchDegradesWhenEmbedderFails(t *testing.T) {
	store := fakeItemSource{items: items(domain.Item{ID: "a", Content: "- run make test"})}
	resolve := func(context.Context) (Embedder, error) { return fakeEmbedder{err: errors.New("no model")}, nil }
	s := NewSearcher(store, resolve)
	got, err := s.Search(context.Background(), domain.ScopeProject, "/repo", "run the tests", 5)
	if err != nil {
		t.Fatalf("embed failure must not fail the search: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("degraded search = %+v, want keyword hit a", got)
	}
}

func TestSearchFusesVectorMatchWithoutKeywordOverlap(t *testing.T) {
	// "b" shares no query terms but is the nearest vector — fusion must surface it.
	store := fakeItemSource{items: items(
		domain.Item{ID: "a", Content: "- unrelated note about tabs", Embedding: []float32{0, 1}},
		domain.Item{ID: "b", Content: "- the build pipeline lives in ci", Embedding: []float32{1, 0}},
	)}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{vectors: map[string][]float32{"where is the pipeline": {1, 0}}}, nil
	}
	s := NewSearcher(store, resolve)
	got, err := s.Search(context.Background(), domain.ScopeProject, "/repo", "where is the pipeline", 2)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range got {
		if item.ID == "b" {
			found = true
		}
	}
	if !found {
		t.Fatalf("vector match b not surfaced: %+v", got)
	}
}

func TestSearchDoesNotReuseCorpusVectorsFromAnotherEmbeddingSpace(t *testing.T) {
	// The persisted vectors were produced by the previous role and rank a first.
	// The current role gives the same-dimensional space different semantics: b
	// is now the nearest item. Reusing the unlabelled cache silently returns the
	// wrong memory instead of refreshing it or degrading to keyword ranking.
	store := fakeItemSource{items: items(
		domain.Item{ID: "a", Content: "alpha memory", Embedding: []float32{1, 0}},
		domain.Item{ID: "b", Content: "beta memory", Embedding: []float32{0, 1}},
	)}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{
			id: "provider:new-space",
			vectors: map[string][]float32{
				"find the target": {1, 0},
				"alpha memory":    {0, 1},
				"beta memory":     {1, 0},
			},
		}, nil
	}
	s := NewSearcher(store, resolve)
	got, err := s.Search(t.Context(), domain.ScopeProject, "/repo", "find the target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("search after embedding-role change = %+v, want item b from the current vector space", got)
	}
}

func TestSearchEmptyCorpus(t *testing.T) {
	s := NewSearcher(fakeItemSource{}, nil)
	got, err := s.Search(context.Background(), domain.ScopeProject, "/repo", "anything", 5)
	if err != nil || got != nil {
		t.Fatalf("empty corpus search = (%+v, %v)", got, err)
	}
}
