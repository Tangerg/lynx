package rag_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

// fakeVectorRetriever captures the request the retriever issues so
// tests can assert that filters / topK / minScore are wired through.
type fakeVectorRetriever struct {
	got *vectorstore.RetrievalRequest
	err error
}

func (f *fakeVectorRetriever) Retrieve(_ context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}
	doc, _ := document.NewDocument("hit", nil)
	return []*document.Document{doc}, nil
}

func TestNewVectorStoreDocumentRetriever_RejectsInvalidConfig(t *testing.T) {
	if _, err := rag.NewVectorStoreDocumentRetriever(nil); err == nil {
		t.Fatal("nil config must error")
	}
	if _, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{}); err == nil {
		t.Fatal("missing VectorStore must error")
	}
	if _, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: &fakeVectorRetriever{},
		MinScore:    1.5,
	}); err == nil {
		t.Fatal("out-of-range MinScore must error")
	}
}

func TestVectorStoreDocumentRetriever_AppliesTopKAndMinScore(t *testing.T) {
	store := &fakeVectorRetriever{}
	r, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
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

func TestVectorStoreDocumentRetriever_PerQueryFilterOverridesFunc(t *testing.T) {
	store := &fakeVectorRetriever{}
	funcCalls := 0

	r, err := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: store,
		FilterFunc: func(_ context.Context, _ map[string]any) (ast.Expr, error) {
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
	q.Set(rag.FilterExprKey, parsed)

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

func TestVectorStoreDocumentRetriever_StringFilterIsParsed(t *testing.T) {
	store := &fakeVectorRetriever{}
	r, _ := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: store,
	})

	q, _ := rag.NewQuery("hi")
	q.Set(rag.FilterExprKey, `year >= 2020`)

	if _, err := r.Retrieve(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if store.got.Filter == nil {
		t.Fatal("string filter was not parsed and threaded through")
	}
}

func TestVectorStoreDocumentRetriever_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	store := &fakeVectorRetriever{err: want}
	r, _ := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{VectorStore: store})

	q, _ := rag.NewQuery("hi")
	if _, err := r.Retrieve(context.Background(), q); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestVectorStoreDocumentRetriever_NilQuery(t *testing.T) {
	r, _ := rag.NewVectorStoreDocumentRetriever(&rag.VectorStoreDocumentRetrieverConfig{
		VectorStore: &fakeVectorRetriever{},
	})
	if _, err := r.Retrieve(context.Background(), nil); err == nil {
		t.Fatal("nil query must error")
	}
}
