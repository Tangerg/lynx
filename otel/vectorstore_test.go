package otel_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	lynxotel "github.com/Tangerg/lynx/otel"
)

type indexerFunc func(context.Context, []*document.Document) error

func (f indexerFunc) Add(ctx context.Context, docs []*document.Document) error {
	return f(ctx, docs)
}

type searcherFunc func(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error)

func (f searcherFunc) Search(ctx context.Context, request vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	return f(ctx, request)
}

func newVectorStoreMiddleware(t *testing.T) (*lynxotel.VectorStoreMiddleware, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	middleware, err := lynxotel.NewVectorStore(lynxotel.VectorStoreConfig{
		System:         "  Qdrant  ",
		Collection:     "knowledge",
		Namespace:      "tenant",
		TracerProvider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return middleware, recorder
}

func TestNewVectorStoreRequiresSystem(t *testing.T) {
	if _, err := lynxotel.NewVectorStore(lynxotel.VectorStoreConfig{}); !errors.Is(err, lynxotel.ErrInvalidVectorStoreConfig) {
		t.Fatalf("NewVectorStore() error = %v, want ErrInvalidVectorStoreConfig", err)
	}
	var provider *sdktrace.TracerProvider
	if _, err := lynxotel.NewVectorStore(lynxotel.VectorStoreConfig{System: "qdrant", TracerProvider: provider}); err != nil {
		t.Fatalf("typed nil tracer provider must use global default: %v", err)
	}
}

func TestVectorStoreMiddlewarePreservesMissingCapabilities(t *testing.T) {
	middleware, _ := newVectorStoreMiddleware(t)
	var indexer indexerFunc
	var searcher searcherFunc
	var idDeleter vectorstore.IDDeleter
	var filterDeleter vectorstore.FilterDeleter
	if middleware.Index(indexer) != nil {
		t.Fatal("Index synthesized an indexer capability")
	}
	if middleware.Search(searcher) != nil {
		t.Fatal("Search synthesized a searcher capability")
	}
	if middleware.DeleteIDs(idDeleter) != nil {
		t.Fatal("DeleteIDs synthesized an ID-deleter capability")
	}
	if middleware.DeleteWhere(filterDeleter) != nil {
		t.Fatal("DeleteWhere synthesized a filter-deleter capability")
	}
}

func TestVectorStoreIndexPreservesNarrowCapabilityAndError(t *testing.T) {
	middleware, recorder := newVectorStoreMiddleware(t)
	want := errors.New("write failed")
	var sawSpan bool
	wrapped := middleware.Index(indexerFunc(func(ctx context.Context, docs []*document.Document) error {
		sawSpan = trace.SpanFromContext(ctx).SpanContext().IsValid() && len(docs) == 2
		return want
	}))
	if _, leaked := wrapped.(vectorstore.Searcher); leaked {
		t.Fatal("Indexer decorator leaked Searcher capability")
	}
	err := wrapped.Add(t.Context(), []*document.Document{{ID: "one", Text: "one"}, {ID: "two", Text: "two"}})
	if !errors.Is(err, want) || !sawSpan {
		t.Fatalf("Add() error/span = %v/%t", err, sawSpan)
	}

	span := recorder.Ended()[0]
	if span.Name() != "add knowledge" || span.SpanKind() != trace.SpanKindClient || span.Status().Code != codes.Error {
		t.Fatalf("span name/kind/status = %q/%v/%v", span.Name(), span.SpanKind(), span.Status())
	}
	attrs := spanAttributes(t, span)
	assertStringAttr(t, attrs, "db.system.name", "qdrant")
	assertStringAttr(t, attrs, "db.operation.name", "add")
	assertStringAttr(t, attrs, "db.collection.name", "knowledge")
	assertStringAttr(t, attrs, "db.namespace", "tenant")
	if got := attrs["db.operation.batch.size"].AsInt64(); got != 2 {
		t.Fatalf("db.operation.batch.size = %d, want 2", got)
	}
}

func TestVectorStoreSearchPreservesMatchesAndRecordsCount(t *testing.T) {
	middleware, recorder := newVectorStoreMiddleware(t)
	want := []vectorstore.Match{{Document: &document.Document{ID: "one", Text: "one"}, Score: 0.9}}
	wrapped := middleware.Search(searcherFunc(func(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error) {
		return want, nil
	}))
	if _, leaked := wrapped.(vectorstore.Indexer); leaked {
		t.Fatal("Searcher decorator leaked Indexer capability")
	}
	got, err := wrapped.Search(t.Context(), vectorstore.SearchRequest{Query: "query", TopK: 1})
	if err != nil || &got[0] != &want[0] {
		t.Fatalf("Search() = %#v, %v", got, err)
	}
	attrs := spanAttributes(t, recorder.Ended()[0])
	if count := attrs["db.response.returned_rows"].AsInt64(); count != 1 {
		t.Fatalf("db.response.returned_rows = %d, want 1", count)
	}
	if topK := attrs["db.vector.query.top_k"].AsInt64(); topK != 1 {
		t.Fatalf("db.vector.query.top_k = %d, want 1", topK)
	}
	if threshold := attrs["db.vector.query.similarity_threshold"].AsFloat64(); threshold != 0 {
		t.Fatalf("db.vector.query.similarity_threshold = %v, want 0", threshold)
	}
}
