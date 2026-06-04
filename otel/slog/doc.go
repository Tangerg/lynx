// Package slog sinks all three OpenTelemetry signals — Traces, Metrics,
// and Logs — into a log/slog logger, so every span, metric, and log line
// from any Lynx module lands in one structured stream correlated by
// trace_id / span_id.
//
// Three exporters, one per signal:
//
//   - [SpanExporter]   (sdktrace.SpanExporter)  — install via WithSyncer / WithBatcher
//   - [MetricExporter] (sdkmetric.Exporter)     — install via a PeriodicReader
//   - [LogExporter]    (sdklog.Exporter)         — install via a LoggerProvider processor
//
// This is for local development and debugging, where forwarding to a full
// backend (Jaeger, Tempo, Datadog, ...) would be overkill. Routing logs
// through OTel (rather than writing slog directly) is deliberate: it makes
// logs as backend-swappable as traces/metrics — a production build swaps
// each exporter to OTLP with zero business-code change.
//
// # Usage
//
//	import (
//	    stdslog "log/slog"
//	    "go.opentelemetry.io/otel"
//	    sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	    "github.com/Tangerg/lynx/otel/slog"
//	)
//
//	tp := sdktrace.NewTracerProvider(
//	    sdktrace.WithSyncer(slog.NewSpanExporter(stdslog.Default())),
//	)
//	otel.SetTracerProvider(tp)
//	defer tp.Shutdown(context.Background())
//
// See lyra/cmd/lyra/observability.go for the full triad wiring (the log
// path goes through the contrib otelslog bridge → a LoggerProvider →
// [LogExporter]).
//
// Note: this package is named `slog` to match the otel/<backend>
// convention. Callers that also import the standard library's `log/slog`
// must alias one of them (commonly `stdslog "log/slog"`) to avoid the
// name collision.
//
// For production use, prefer OTLP exporters to a real backend; these
// exporters are intended for local visibility, not long-term storage.
package slog
