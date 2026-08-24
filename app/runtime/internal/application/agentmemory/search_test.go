package agentmemory

import (
	"context"
	"errors"
	"slices"
	"testing"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

type fakeItemSource struct {
	items    []domain.Item
	err      error
	cacheErr error
	updates  []domain.EmbeddingUpdate
}

func (f *fakeItemSource) SearchCorpus(context.Context, string) ([]domain.Item, error) {
	return slices.Clone(f.items), f.err
}

func (f *fakeItemSource) SetEmbeddings(_ context.Context, updates []domain.EmbeddingUpdate) error {
	f.updates = append(f.updates, updates...)
	if f.cacheErr != nil {
		return f.cacheErr
	}
	for _, update := range updates {
		for index := range f.items {
			if f.items[index].ID != update.ItemID || domain.Digest(f.items[index].Content) != update.ContentDigest {
				continue
			}
			f.items[index].EmbeddingSpace = update.Space
			f.items[index].Embedding = slices.Clone(update.Vector)
		}
	}
	return nil
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
	store := &fakeItemSource{items: items(
		domain.Item{ID: "a", Content: "- run make test to build"},
		domain.Item{ID: "b", Content: "- prefer tabs over spaces"},
		domain.Item{ID: "c", Content: "- deploy with kubectl apply"},
	)}
	s := NewSearcher(store, nil)
	got, err := s.Search(context.Background(), "/repo", "how do we run tests", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("keyword search = %+v, want just item a", got)
	}
}

func TestSearchDegradesWhenEmbedderFails(t *testing.T) {
	store := &fakeItemSource{items: items(domain.Item{ID: "a", Content: "- run make test"})}
	resolve := func(context.Context) (Embedder, error) { return fakeEmbedder{err: errors.New("no model")}, nil }
	s := NewSearcher(store, resolve)
	got, err := s.Search(context.Background(), "/repo", "run the tests", 5)
	if err != nil {
		t.Fatalf("embed failure must not fail the search: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("degraded search = %+v, want keyword hit a", got)
	}
}

func TestSearchFusesVectorMatchWithoutKeywordOverlap(t *testing.T) {
	// "b" shares no query terms but is the nearest vector — fusion must surface it.
	store := &fakeItemSource{items: items(
		domain.Item{ID: "a", Content: "- unrelated note about tabs", EmbeddingSpace: "fake", Embedding: []float32{0, 1}},
		domain.Item{ID: "b", Content: "- the build pipeline lives in ci", EmbeddingSpace: "fake", Embedding: []float32{1, 0}},
	)}
	resolve := func(context.Context) (Embedder, error) {
		return fakeEmbedder{vectors: map[string][]float32{"where is the pipeline": {1, 0}}}, nil
	}
	s := NewSearcher(store, resolve)
	got, err := s.Search(context.Background(), "/repo", "where is the pipeline", 2)
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
	store := &fakeItemSource{cacheErr: errors.New("cache write lost"), items: items(
		domain.Item{ID: "a", Content: "alpha memory", EmbeddingSpace: "provider:old-space", Embedding: []float32{1, 0}},
		domain.Item{ID: "b", Content: "beta memory", EmbeddingSpace: "provider:old-space", Embedding: []float32{0, 1}},
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
	got, err := s.Search(t.Context(), "/repo", "find the target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("search after embedding-role change = %+v, want item b from the current vector space", got)
	}
	if len(store.updates) != 2 || store.updates[0].Space != "provider:new-space" {
		t.Fatalf("cache updates = %+v, want both items bound to the current space", store.updates)
	}
}

func TestSearchEmptyCorpus(t *testing.T) {
	s := NewSearcher(&fakeItemSource{}, nil)
	got, err := s.Search(context.Background(), "/repo", "anything", 5)
	if err != nil || got != nil {
		t.Fatalf("empty corpus search = (%+v, %v)", got, err)
	}
}
