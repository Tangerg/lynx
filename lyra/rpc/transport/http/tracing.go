package http

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the package-wide OTel tracer. Names follow the
// `lynx/lyra/...` convention used elsewhere in the monorepo so
// trace dashboards can bucket lyra HTTP transport spans
// separately from other lyra-side instrumentation.
var tracer = otel.Tracer("lynx/lyra/rpc/transport/http")

// recordError emits a short-lived span carrying err, parented on ctx so
// the error chains onto the caller's trace. The span ends immediately;
// backends see it as a point-event with stack + attributes.
//
// Callers pass the live request / goroutine ctx so the span links to the
// enclosing trace — the chain is never silently swapped to a fresh root
// inside here. A genuinely detached point-event (no enclosing trace) may
// pass context.Background() explicitly at the call site.
//
// attrs supplies any context the operator needs to correlate the
// failure — typically run_id, event_id, or method name.
func recordError(ctx context.Context, spanName string, err error, attrs ...attribute.KeyValue) {
	if err == nil {
		return
	}
	_, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(attrs...),
	)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}
