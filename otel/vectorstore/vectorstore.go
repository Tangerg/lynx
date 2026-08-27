// Package vectorstore instruments Core vector-store capabilities with OpenTelemetry.
package vectorstore

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

	"github.com/samber/lo"

	corevectorstore "github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

const instrumentationName = "github.com/Tangerg/scope/otel/vectorstore"

// ErrInvalidConfig reports a missing database-system identity.
var ErrInvalidConfig = errors.New("otel/vectorstore: invalid config")

// MiddlewareConfig identifies the database observed by vector-store
// instrumentation. System is the OpenTelemetry db.system.name value.
// Collection and Namespace identify the provider-native storage target.
type MiddlewareConfig struct {
	System         string
	Collection     string
	Namespace      string
	TracerProvider trace.TracerProvider
}

// Validate verifies the database-system identity required by vector-store
// instrumentation.
func (c MiddlewareConfig) Validate() error {
	if strings.TrimSpace(c.System) == "" {
		return fmt.Errorf("%w: system is required", ErrInvalidConfig)
	}
	return nil
}

// Middleware instruments vector-store capabilities without changing
// the capability set of the wrapped value.
type Middleware struct {
	system     string
	collection string
	namespace  string
	tracer     trace.Tracer
}

// NewMiddleware constructs vector-store instrumentation. It belongs at the
// composition root; providers remain unaware of OpenTelemetry.
func NewMiddleware(config MiddlewareConfig) (Middleware, error) {
	if err := config.Validate(); err != nil {
		return Middleware{}, err
	}
	system := strings.ToLower(strings.TrimSpace(config.System))

	tracerProvider := config.TracerProvider
	if lo.IsNil(tracerProvider) {
		tracerProvider = apiotel.GetTracerProvider()
	}
	return Middleware{
		system:     system,
		collection: strings.TrimSpace(config.Collection),
		namespace:  strings.TrimSpace(config.Namespace),
		tracer:     tracerProvider.Tracer(instrumentationName),
	}, nil
}

// Index instruments only the [corevectorstore.Indexer] capability.
func (m Middleware) Index(next corevectorstore.Indexer) corevectorstore.Indexer {
	if lo.IsNil(next) {
		return nil
	}
	return indexer{middleware: m, next: next}
}

// Search instruments only the [corevectorstore.Searcher] capability.
func (m Middleware) Search(next corevectorstore.Searcher) corevectorstore.Searcher {
	if lo.IsNil(next) {
		return nil
	}
	return searcher{middleware: m, next: next}
}

// DeleteIDs instruments only the [corevectorstore.IDDeleter] capability.
func (m Middleware) DeleteIDs(next corevectorstore.IDDeleter) corevectorstore.IDDeleter {
	if lo.IsNil(next) {
		return nil
	}
	return idDeleter{middleware: m, next: next}
}

// DeleteWhere instruments only the [corevectorstore.FilterDeleter] capability.
func (m Middleware) DeleteWhere(next corevectorstore.FilterDeleter) corevectorstore.FilterDeleter {
	if lo.IsNil(next) {
		return nil
	}
	return filterDeleter{middleware: m, next: next}
}

func (m Middleware) start(
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

type indexer struct {
	middleware Middleware
	next       corevectorstore.Indexer
}

func (i indexer) Index(ctx context.Context, request *corevectorstore.IndexRequest) error {
	extra := make([]attribute.KeyValue, 0, 1)
	if request != nil && len(request.Documents) > 1 {
		extra = append(extra, semconv.DBOperationBatchSizeKey.Int(len(request.Documents)))
	}
	ctx, span := i.middleware.start(ctx, "index", extra...)
	err := i.next.Index(ctx, request)
	finishVectorStoreSpan(span, err)
	return err
}

type searcher struct {
	middleware Middleware
	next       corevectorstore.Searcher
}

func (s searcher) Search(ctx context.Context, request *corevectorstore.SearchRequest) (*corevectorstore.SearchResponse, error) {
	var topK int
	var minScore float64
	if request != nil {
		topK = request.Options.TopK
		minScore = request.Options.MinScore.Float64()
	}
	ctx, span := s.middleware.start(ctx, "search",
		attribute.Int("db.vector.query.top_k", topK),
		attribute.Float64("db.vector.query.similarity_threshold", minScore),
	)
	response, err := s.next.Search(ctx, request)
	if err == nil && response != nil {
		span.SetAttributes(semconv.DBResponseReturnedRowsKey.Int(len(response.Results)))
	}
	finishVectorStoreSpan(span, err)
	return response, err
}

type idDeleter struct {
	middleware Middleware
	next       corevectorstore.IDDeleter
}

func (i idDeleter) DeleteIDs(ctx context.Context, ids []string) error {
	extra := make([]attribute.KeyValue, 0, 1)
	if len(ids) > 1 {
		extra = append(extra, semconv.DBOperationBatchSizeKey.Int(len(ids)))
	}
	ctx, span := i.middleware.start(ctx, "delete_ids", extra...)
	err := i.next.DeleteIDs(ctx, ids)
	finishVectorStoreSpan(span, err)
	return err
}

type filterDeleter struct {
	middleware Middleware
	next       corevectorstore.FilterDeleter
}

func (f filterDeleter) DeleteWhere(ctx context.Context, predicate filter.Predicate) error {
	ctx, span := f.middleware.start(ctx, "delete_where")
	err := f.next.DeleteWhere(ctx, predicate)
	finishVectorStoreSpan(span, err)
	return err
}
