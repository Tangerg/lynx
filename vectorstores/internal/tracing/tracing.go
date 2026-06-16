package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
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
		attrs = append(attrs, attribute.Int(attrDocCount, inputCount))
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
	Finish(span, err, attribute.Int(attrDocCount, docCount))
}

// Decorator wraps any [vectorstore.Store] implementation with the
// DB-semconv span emission described above. Useful as the universal
// escape hatch for providers whose Create/Retrieve/Delete bodies
// haven't been hand-instrumented yet — `tracing.WrapStore(inner,
// "system")` gives instant coverage without touching the provider's
// source.
//
// Inline instrumentation (named-return + deferred Finish) remains
// the preferred pattern for providers that want the span context
// available inside the method body (so sub-operations can attach as
// children). The decorator only emits one outer span per call; any
// sub-calls (embedding, batching) run in the unmodified parent
// context.
type Decorator struct {
	inner  vectorstore.Store
	system string
}

// WrapStore wraps inner with span emission tagged `system`.
func WrapStore(inner vectorstore.Store, system string) *Decorator {
	return &Decorator{inner: inner, system: system}
}

// Retrieve emits a `db.vector.retrieve <system>` span around the
// inner call.
func (d *Decorator) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	topK, minScore := 0, 0.0
	if req != nil {
		topK = req.TopK
		minScore = req.MinScore
	}
	ctx, span := StartRetrieve(ctx, d.system, topK, minScore)
	defer func() { RecordRetrieveResult(span, err, len(docs)) }()
	return d.inner.Retrieve(ctx, req)
}

// Create emits a `db.vector.create <system>` span around the inner
// call.
func (d *Decorator) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	inputCount := 0
	if req != nil {
		inputCount = len(req.Documents)
	}
	ctx, span := StartCreate(ctx, d.system, inputCount)
	defer func() { Finish(span, err) }()
	return d.inner.Create(ctx, req)
}

// Delete emits a `db.vector.delete <system>` span around the inner
// call.
func (d *Decorator) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	ctx, span := StartDelete(ctx, d.system)
	defer func() { Finish(span, err) }()
	return d.inner.Delete(ctx, req)
}

// Metadata forwards to the wrapped store.
func (d *Decorator) Metadata() vectorstore.StoreMetadata {
	return d.inner.Metadata()
}
