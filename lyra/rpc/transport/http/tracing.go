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

// recordError emits a short-lived span carrying err. Use for code
// paths that have no enclosing request span (e.g. the SSE event
// pump goroutine, where the request that triggered it has already
// returned). The span ends immediately; backends see it as a
// point-event with stack + attributes.
//
// attrs supplies any context the operator needs to correlate the
// failure — typically run_id, event_id, or method name.
func recordError(spanName string, err error, attrs ...attribute.KeyValue) {
	if err == nil {
		return
	}
	_, span := tracer.Start(context.Background(), spanName,
		trace.WithAttributes(attrs...),
	)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}
