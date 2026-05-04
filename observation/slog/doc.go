// Package slog provides an OpenTelemetry trace SpanExporter that writes
// spans to a log/slog logger.
//
// This is primarily useful for local development and debugging, where
// forwarding spans to a full tracing backend (Jaeger, Tempo, Datadog, ...)
// would be overkill. It lets every Lynx span appear in the same structured
// log stream as business logs, correlated via trace_id / span_id / parent_span_id.
//
// # Usage
//
//	import (
//	    stdslog "log/slog"
//	    "go.opentelemetry.io/otel"
//	    sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	    "github.com/Tangerg/lynx/observation/slog"
//	)
//
//	tp := sdktrace.NewTracerProvider(
//	    sdktrace.WithSyncer(slog.NewExporter(stdslog.Default())),
//	)
//	otel.SetTracerProvider(tp)
//	defer tp.Shutdown(context.Background())
//
// Note: this package is named `slog` to match the otelbridge/<backend>
// convention. Callers that also import the standard library's `log/slog`
// must alias one of them (commonly `stdslog "log/slog"`) to avoid the
// name collision.
//
// After wiring up the exporter, every span produced by Lynx (chat.Call,
// rag.Pipeline, vectorstore operations, agent ticks, tool invocations, ...)
// is emitted as an slog record with level Info (or Error if the span status
// is Error).
//
// For production use, prefer an OTLP exporter to a real tracing backend;
// this exporter is intended for local visibility, not long-term storage.
package slog
