package rag_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/rag"
)

// fakeVectorSearcher captures the request the retriever issues so
// tests can assert that filters / topK / minScore are wired through.
type fakeVectorSearcher struct {
	got vectorstore.SearchRequest
	err error
}

func (f *fakeVectorSearcher) Search(_ context.Context, req vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}
	doc, _ := document.NewDocument("hit", nil)
	return []vectorstore.Match{{Document: doc, Score: 0.75}}, nil
}

func TestNewVectorStoreRetrieverRejectsInvalidConfig(t *testing.T) {
	if _, err := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{}); err == nil {
		t.Fatal("nil config must error")
	}
	if _, err := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{}); err == nil {
		t.Fatal("missing VectorStore must error")
	}
	if _, err := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{
		VectorStore: &fakeVectorSearcher{},
		MinScore:    1.5,
	}); err == nil {
		t.Fatal("out-of-range MinScore must error")
	}
}

func TestRetrieverAppliesTopKAndMinScore(t *testing.T) {
	store := &fakeVectorSearcher{}
	r, err := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{
		VectorStore: store,
		TopK:        7,
		MinScore:    0.42,
	})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("hi")
	if _, err := r.Retrieve(context.Background(), q); err != nil {
		t.Fatal(err)
	}

	if store.got.TopK != 7 {
		t.Fatalf("TopK = %d, want 7", store.got.TopK)
	}
	if store.got.MinScore != 0.42 {
		t.Fatalf("MinScore = %f, want 0.42", store.got.MinScore)
	}
}

func TestRetrieverPerQueryFilterOverridesFunc(t *testing.T) {
	store := &fakeVectorSearcher{}
	funcCalls := 0

	r, err := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{
		VectorStore: store,
		FilterFunc: func(_ context.Context, _ map[string]any) (filter.Expr, error) {
			funcCalls++
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("hi")
	parsed, err := filter.Parse(`category == 'tech'`)
	if err != nil {
		t.Fatal(err)
	}
	q.Set(rag.VectorStoreFilterKey, parsed)

	if _, err := r.Retrieve(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if funcCalls != 0 {
		t.Fatal("per-query filter must override FilterFunc")
	}
	if store.got.Filter == nil {
		t.Fatal("filter was not threaded into the retrieval request")
	}
}

func TestRetrieverStringFilterIsParsed(t *testing.T) {
	store := &fakeVectorSearcher{}
	r, _ := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{
		VectorStore: store,
	})

	q, _ := rag.NewQuery("hi")
	q.Set(rag.VectorStoreFilterKey, `year >= 2020`)

	if _, err := r.Retrieve(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if store.got.Filter == nil {
		t.Fatal("string filter was not parsed and threaded through")
	}
}

func TestRetrieverPropagatesError(t *testing.T) {
	want := errors.New("boom")
	store := &fakeVectorSearcher{err: want}
	r, _ := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{VectorStore: store})

	q, _ := rag.NewQuery("hi")
	if _, err := r.Retrieve(context.Background(), q); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestRetrieverNilQuery(t *testing.T) {
	r, _ := rag.NewVectorStoreRetriever(rag.VectorStoreConfig{
		VectorStore: &fakeVectorSearcher{},
	})
	if _, err := r.Retrieve(context.Background(), nil); err == nil {
		t.Fatal("nil query must error")
	}
}
