package otel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apiotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// ErrInvalidVectorStoreConfig reports a missing database-system identity.
var ErrInvalidVectorStoreConfig = errors.New("otel: invalid vector store config")

// VectorStoreConfig identifies the database observed by vector-store
// instrumentation. System is the OpenTelemetry db.system.name value.
// Collection and Namespace identify the provider-native storage target.
type VectorStoreConfig struct {
	System         string
	Collection     string
	Namespace      string
	TracerProvider trace.TracerProvider
}

// VectorStoreMiddleware instruments vector-store capabilities without changing
// the capability set of the wrapped value.
type VectorStoreMiddleware struct {
	system     string
	collection string
	namespace  string
	tracer     trace.Tracer
}

// NewVectorStore constructs vector-store instrumentation. It belongs at the
// composition root; providers remain unaware of OpenTelemetry.
func NewVectorStore(config VectorStoreConfig) (*VectorStoreMiddleware, error) {
	system := strings.ToLower(strings.TrimSpace(config.System))
	if system == "" {
		return nil, fmt.Errorf("%w: System is required", ErrInvalidVectorStoreConfig)
	}

	tracerProvider := config.TracerProvider
	if tracerProvider == nil {
		tracerProvider = apiotel.GetTracerProvider()
	}
	return &VectorStoreMiddleware{
		system:     system,
		collection: strings.TrimSpace(config.Collection),
		namespace:  strings.TrimSpace(config.Namespace),
		tracer:     tracerProvider.Tracer(instrumentationName),
	}, nil
}

// Index instruments only the [vectorstore.Indexer] capability.
func (m *VectorStoreMiddleware) Index(next vectorstore.Indexer) vectorstore.Indexer {
	return indexerFunc(func(ctx context.Context, docs []*document.Document) error {
		extra := make([]attribute.KeyValue, 0, 1)
		if len(docs) > 1 {
			extra = append(extra, semconv.DBOperationBatchSizeKey.Int(len(docs)))
		}
		ctx, span := m.start(ctx, "add", extra...)
		err := next.Add(ctx, docs)
		finishVectorStoreSpan(span, err)
		return err
	})
}

// Search instruments only the [vectorstore.Searcher] capability.
func (m *VectorStoreMiddleware) Search(next vectorstore.Searcher) vectorstore.Searcher {
	return searcherFunc(func(ctx context.Context, request vectorstore.SearchRequest) ([]vectorstore.Match, error) {
		ctx, span := m.start(ctx, "search",
			attribute.Int("db.vector.query.top_k", request.TopK),
			attribute.Float64("db.vector.query.similarity_threshold", request.MinScore),
		)
		matches, err := next.Search(ctx, request)
		if err == nil {
			span.SetAttributes(semconv.DBResponseReturnedRowsKey.Int(len(matches)))
		}
		finishVectorStoreSpan(span, err)
		return matches, err
	})
}

// DeleteIDs instruments only the [vectorstore.IDDeleter] capability.
func (m *VectorStoreMiddleware) DeleteIDs(next vectorstore.IDDeleter) vectorstore.IDDeleter {
	return idDeleterFunc(func(ctx context.Context, ids []string) error {
		extra := make([]attribute.KeyValue, 0, 1)
		if len(ids) > 1 {
			extra = append(extra, semconv.DBOperationBatchSizeKey.Int(len(ids)))
		}
		ctx, span := m.start(ctx, "delete_ids", extra...)
		err := next.DeleteIDs(ctx, ids)
		finishVectorStoreSpan(span, err)
		return err
	})
}

// DeleteWhere instruments only the [vectorstore.FilterDeleter] capability.
func (m *VectorStoreMiddleware) DeleteWhere(next vectorstore.FilterDeleter) vectorstore.FilterDeleter {
	return filterDeleterFunc(func(ctx context.Context, predicate filter.Predicate) error {
		ctx, span := m.start(ctx, "delete_where")
		err := next.DeleteWhere(ctx, predicate)
		finishVectorStoreSpan(span, err)
		return err
	})
}

func (m *VectorStoreMiddleware) start(
	ctx context.Context,
	operation string,
	extra ...attribute.KeyValue,
) (context.Context, trace.Span) {
	attrs := make([]attribute.KeyValue, 0, 3+len(extra))
	attrs = append(attrs,
		semconv.DBSystemNameKey.String(m.system),
		semconv.DBOperationNameKey.String(operation),
	)
	target := ""
	if m.collection != "" {
		attrs = append(attrs, semconv.DBCollectionNameKey.String(m.collection))
		target = m.collection
	}
	if m.namespace != "" {
		attrs = append(attrs, semconv.DBNamespaceKey.String(m.namespace))
		if target == "" {
			target = m.namespace
		}
	}
	attrs = append(attrs, extra...)

	name := operation
	if target != "" {
		name += " " + target
	}
	return m.tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

func finishVectorStoreSpan(span trace.Span, err error) {
	defer span.End()
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type indexerFunc func(context.Context, []*document.Document) error

func (f indexerFunc) Add(ctx context.Context, docs []*document.Document) error {
	return f(ctx, docs)
}

type searcherFunc func(context.Context, vectorstore.SearchRequest) ([]vectorstore.Match, error)

func (f searcherFunc) Search(ctx context.Context, request vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	return f(ctx, request)
}

type idDeleterFunc func(context.Context, []string) error

func (f idDeleterFunc) DeleteIDs(ctx context.Context, ids []string) error {
	return f(ctx, ids)
}

type filterDeleterFunc func(context.Context, filter.Predicate) error

func (f filterDeleterFunc) DeleteWhere(ctx context.Context, predicate filter.Predicate) error {
	return f(ctx, predicate)
}
