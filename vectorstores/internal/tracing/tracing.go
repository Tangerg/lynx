package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTel DB semantic-convention attribute keys emitted on every vector
// store span. The one non-semconv key (rag.doc_count) uses a bare
// domain prefix — no brand (no lynx.*) — per the observability convention.
const (
	attrDBSystem                  = "db.system"
	attrDBOperationName           = "db.operation.name"
	attrDBVectorTopK              = "db.vector.query.top_k"
	attrDBVectorSimilarityMinimum = "db.vector.query.similarity_threshold"
	attrDocCount                  = "rag.doc_count"
)

// tracerFor returns the per-provider tracer. Tracer names follow
// `lynx/vectorstore/<system>` so backends that bucket by tracer name
// can distinguish providers without parsing attributes.
func tracerFor(system string) trace.Tracer {
	return otel.Tracer("lynx/vectorstore/" + system)
}

// StartSearch opens a span for [vectorstore.Searcher.Search]. The
// span name is `db.vector.search <system>` (e.g.
// `db.vector.search qdrant`). Top-k and min-score are stamped
// when provided.
func StartSearch(ctx context.Context, system string, topK int, minScore float64) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, "search"),
	}
	if topK > 0 {
		attrs = append(attrs, attribute.Int(attrDBVectorTopK, topK))
	}
	if minScore > 0 {
		attrs = append(attrs, attribute.Float64(attrDBVectorSimilarityMinimum, minScore))
	}
	return tracerFor(system).Start(ctx, "db.vector.search "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartAdd opens a span for [vectorstore.Indexer.Add]. inputCount
// records the number of documents in the batch.
func StartAdd(ctx context.Context, system string, inputCount int) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, "add"),
	}
	if inputCount > 0 {
		attrs = append(attrs, attribute.Int(attrDocCount, inputCount))
	}
	return tracerFor(system).Start(ctx, "db.vector.add "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartDelete opens a span for an ID or filter deletion capability.
func StartDelete(ctx context.Context, system string) (context.Context, trace.Span) {
	return tracerFor(system).Start(ctx, "db.vector.delete "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(attrDBSystem, system),
			attribute.String(attrDBOperationName, "delete"),
		),
	)
}

// Finish records err on span and ends it. Extra attributes (for example,
// search match count) are stamped before End.
func Finish(span trace.Span, err error, extra ...attribute.KeyValue) {
	if len(extra) > 0 {
		span.SetAttributes(extra...)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// RecordSearchResult stamps the match count onto the span before ending it.
func RecordSearchResult(span trace.Span, err error, docCount int) {
	Finish(span, err, attribute.Int(attrDocCount, docCount))
}
