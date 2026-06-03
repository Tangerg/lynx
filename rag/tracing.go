package rag

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ragTracer is the package-level tracer for RAG pipeline span
// emission. Tracer name follows the `lynx/<subsystem>` convention.
// No-op overhead when no TracerProvider is installed — see
// doc/OBSERVABILITY.md §5.
var ragTracer = otel.Tracer("lynx/rag")

// Lynx RAG attribute keys — the GenAI semconv has no RAG-specific
// registry today, so these live under `lynx.rag.*` per
// doc/OBSERVABILITY.md §3.3.
const (
	attrLynxRAGStage      = "lynx.rag.stage"
	attrLynxRAGQueryCount = "lynx.rag.query_count"
	attrLynxRAGDocCount   = "lynx.rag.doc_count"
)

// startStageSpan opens a child span for one RAG pipeline stage. The
// span name is `rag.<stage>` (e.g. `rag.transform`) and the stage is
// also stamped onto the `lynx.rag.stage` attribute so backends that
// surface attribute-based filtering can pivot on it.
//
// Stage names: transform / expand / retrieve / refine / augment.
func startStageSpan(ctx context.Context, stage string) (context.Context, trace.Span) {
	return ragTracer.Start(ctx, "rag."+stage,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrLynxRAGStage, stage)),
	)
}

// finishSpan ends span and records err on the span when non-nil.
// Optional extra attributes (stage-specific counts) are stamped before
// End.
func finishSpan(span trace.Span, err error, extra ...attribute.KeyValue) {
	if len(extra) > 0 {
		span.SetAttributes(extra...)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
