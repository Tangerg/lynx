package vectorstore_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	vectorotel "github.com/Tangerg/lynx/otel/vectorstore"
)

type indexerFunc func(context.Context, *vectorstore.IndexRequest) error

func (i indexerFunc) Index(ctx context.Context, request *vectorstore.IndexRequest) error {
	return i(ctx, request)
}

type searcherFunc func(context.Context, *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error)

func (s searcherFunc) Search(ctx context.Context, request *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error) {
	return s(ctx, request)
}

func newVectorStoreMiddleware(t *testing.T) (vectorotel.Middleware, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	middleware, err := vectorotel.New(vectorotel.Config{
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

func TestNewRequiresSystem(t *testing.T) {
	if _, err := vectorotel.New(vectorotel.Config{}); !errors.Is(err, vectorotel.ErrInvalidConfig) {
		t.Fatalf("NewVectorStore() error = %v, want ErrInvalidVectorStoreConfig", err)
	}
	var provider *sdktrace.TracerProvider
	if _, err := vectorotel.New(vectorotel.Config{System: "qdrant", TracerProvider: provider}); err != nil {
		t.Fatalf("typed nil tracer provider must use global default: %v", err)
	}
}

func TestMiddlewarePreservesMissingCapabilities(t *testing.T) {
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

func TestIndexPreservesNarrowCapabilityAndError(t *testing.T) {
	middleware, recorder := newVectorStoreMiddleware(t)
	want := errors.New("write failed")
	var sawSpan bool
	wrapped := middleware.Index(indexerFunc(func(ctx context.Context, request *vectorstore.IndexRequest) error {
		sawSpan = trace.SpanFromContext(ctx).SpanContext().IsValid() && len(request.Documents) == 2
		return want
	}))
	if _, leaked := wrapped.(vectorstore.Searcher); leaked {
		t.Fatal("Indexer decorator leaked Searcher capability")
	}
	err := wrapped.Index(t.Context(), &vectorstore.IndexRequest{Documents: []*document.Document{{ID: "one", Text: "one"}, {ID: "two", Text: "two"}}})
	if !errors.Is(err, want) || !sawSpan {
		t.Fatalf("Index() error/span = %v/%t", err, sawSpan)
	}

	span := recorder.Ended()[0]
	if span.Name() != "index knowledge" || span.SpanKind() != trace.SpanKindClient || span.Status().Code != codes.Error {
		t.Fatalf("span name/kind/status = %q/%v/%v", span.Name(), span.SpanKind(), span.Status())
	}
	attrs := spanAttributes(t, span)
	assertStringAttr(t, attrs, "db.system.name", "qdrant")
	assertStringAttr(t, attrs, "db.operation.name", "index")
	assertStringAttr(t, attrs, "db.collection.name", "knowledge")
	assertStringAttr(t, attrs, "db.namespace", "tenant")
	if got := attrs["db.operation.batch.size"].AsInt64(); got != 2 {
		t.Fatalf("db.operation.batch.size = %d, want 2", got)
	}
}

func TestSearchPreservesMatchesAndRecordsCount(t *testing.T) {
	middleware, recorder := newVectorStoreMiddleware(t)
	want := &vectorstore.SearchResponse{Results: []*vectorstore.SearchResult{{Document: &document.Document{ID: "one", Text: "one"}, Score: 0.9}}}
	wrapped := middleware.Search(searcherFunc(func(context.Context, *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error) {
		return want, nil
	}))
	if _, leaked := wrapped.(vectorstore.Indexer); leaked {
		t.Fatal("Searcher decorator leaked Indexer capability")
	}
	got, err := wrapped.Search(t.Context(), &vectorstore.SearchRequest{Query: "query", Options: vectorstore.SearchOptions{TopK: 1}})
	if err != nil || got != want {
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

func spanAttributes(t *testing.T, span sdktrace.ReadOnlySpan) map[string]attribute.Value {
	t.Helper()
	values := make(map[string]attribute.Value, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		values[string(attr.Key)] = attr.Value
	}
	return values
}

func assertStringAttr(t *testing.T, attrs map[string]attribute.Value, key, want string) {
	t.Helper()
	if got := attrs[key].AsString(); got != want {
		t.Fatalf("attribute %s = %q, want %q", key, got, want)
	}
}
