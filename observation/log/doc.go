// Package log provides an OpenTelemetry trace SpanExporter that writes
// spans to a standard library *log.Logger.
//
// This is a thinner alternative to the sibling slog exporter for projects
// that have not yet migrated to log/slog, or that wire custom infrastructure
// around the stdlib log package (e.g. a shared log.SetOutput redirect).
//
// # Usage
//
//	import (
//	    stdlog "log"
//	    "go.opentelemetry.io/otel"
//	    sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	    "github.com/Tangerg/lynx/observation/log"
//	)
//
//	tp := sdktrace.NewTracerProvider(
//	    sdktrace.WithSyncer(log.NewExporter(stdlog.Default())),
//	)
//	otel.SetTracerProvider(tp)
//	defer tp.Shutdown(context.Background())
//
// Note: this package is named `log` to match the otelbridge/<backend>
// convention. Callers that also import the standard library's `log`
// must alias one of them (commonly `stdlog "log"`) to avoid the name
// collision.
//
// Each span is rendered on a single line in a key=value style similar to
// logfmt, e.g.:
//
//	2026/04/20 10:30:00 span trace_id=a1b2... span_id=aabb... name="gen_ai.chat" duration=523ms gen_ai.system=openai
//
// Error spans get an "[ERROR]" prefix and include the status description:
//
//	2026/04/20 10:30:00 [ERROR] span (error): timeout trace_id=... name="agent.action" duration=30s
//
// For structured logging, prefer the sibling slog exporter. For production
// tracing backends, use an OTLP exporter.
package log
