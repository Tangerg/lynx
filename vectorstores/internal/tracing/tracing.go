// Package tracing centralises the OTel span emission shared by every
// vector-store provider in this module. Each provider's Create /
// Retrieve / Delete entry point wraps its inner SDK work with the
// helpers here so the GenAI / DB semconv attribute set stays
// consistent across the 27-provider matrix.
//
// Per doc/OBSERVABILITY.md §3.2 the attributes follow the OTel DB
// semconv plus the lynx vector-specific extensions:
//
//	db.system                            — provider id ("qdrant", "pgvector", ...)
//	db.operation.name                    — "create" / "retrieve" / "delete"
//	db.vector.query.top_k                — RetrievalRequest.TopK
//	db.vector.query.similarity_threshold — RetrievalRequest.MinScore
//	lynx.rag.doc_count                   — result size (retrieve) or input size (create)
package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTel DB semantic-convention attribute keys we emit on every vector
// store span. Keys outside this set live under `lynx.*`.
const (
	attrDBSystem                  = "db.system"
	attrDBOperationName           = "db.operation.name"
	attrDBVectorTopK              = "db.vector.query.top_k"
	attrDBVectorSimilarityMinimum = "db.vector.query.similarity_threshold"
	attrLynxRAGDocCount           = "lynx.rag.doc_count"
)

// tracerFor returns the per-provider tracer. Tracer names follow
// `lynx/vectorstore/<system>` so backends that bucket by tracer name
// can distinguish providers without parsing attributes.
func tracerFor(system string) trace.Tracer {
	return otel.Tracer("lynx/vectorstore/" + system)
}

// StartRetrieve opens a span for [vectorstore.Store.Retrieve]. The
// span name is `db.vector.retrieve <system>` (e.g.
// `db.vector.retrieve qdrant`). Top-k and min-score are stamped
// when provided.
func StartRetrieve(ctx context.Context, system string, topK int, minScore float64) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, "retrieve"),
	}
	if topK > 0 {
		attrs = append(attrs, attribute.Int(attrDBVectorTopK, topK))
	}
	if minScore > 0 {
		attrs = append(attrs, attribute.Float64(attrDBVectorSimilarityMinimum, minScore))
	}
	return tracerFor(system).Start(ctx, "db.vector.retrieve "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartCreate opens a span for [vectorstore.Store.Create]. inputCount
// records the number of documents in the batch.
func StartCreate(ctx context.Context, system string, inputCount int) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(attrDBSystem, system),
		attribute.String(attrDBOperationName, "create"),
	}
	if inputCount > 0 {
		attrs = append(attrs, attribute.Int(attrLynxRAGDocCount, inputCount))
	}
	return tracerFor(system).Start(ctx, "db.vector.create "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// StartDelete opens a span for [vectorstore.Store.Delete].
func StartDelete(ctx context.Context, system string) (context.Context, trace.Span) {
	return tracerFor(system).Start(ctx, "db.vector.delete "+system,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String(attrDBSystem, system),
			attribute.String(attrDBOperationName, "delete"),
		),
	)
}

// Finish records err on span and ends it. extra attributes (e.g.
// retrieval doc count) are stamped before End.
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

// RecordRetrieveResult is the convenience overload that stamps the
// retrieved document count onto the span before ending it.
func RecordRetrieveResult(span trace.Span, err error, docCount int) {
	Finish(span, err, attribute.Int(attrLynxRAGDocCount, docCount))
}
