package main

import (
	"context"
	stdslog "log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	otelslog "github.com/Tangerg/lynx/otel/slog"
)

// setupObservability wires the full dev observability triad onto a single
// slog sink, so every span, metric, and log line from any lynx module
// lands in one correlated stream (keyed by trace_id). It is the one place
// the global OTel providers are set — everything else just uses the
// package-static otel.Tracer / otel.Meter / slog accessors, no injection.
//
//   - Logs:    slog.Default() gets a trace-correlating handler (stamps
//              trace_id / span_id from the active span). Level from
//              LYRA_LOG_LEVEL (debug|info|warn|error, default info).
//   - Traces:  global TracerProvider with a synchronous slog span exporter,
//              AlwaysSample (full capture — every request is traced).
//   - Metrics: global MeterProvider with a PeriodicReader flushing to the
//              slog metric exporter.
//   - Context: W3C trace-context + baggage propagator, so a traceparent the
//              frontend sends extends into the backend (full-link tracing).
//
// Returns a shutdown func that flushes + tears down the providers; call it
// on process exit.
func setupObservability(serviceVersion string) func(context.Context) {
	level := parseLogLevel(os.Getenv("LYRA_LOG_LEVEL"))
	base := stdslog.NewTextHandler(os.Stderr, &stdslog.HandlerOptions{Level: level})
	logger := stdslog.New(otelslog.NewHandler(base))
	stdslog.SetDefault(logger)

	res := resource.NewSchemaless(
		attribute.String("service.name", "lyra"),
		attribute.String("service.version", serviceVersion),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(otelslog.NewExporter(logger)),
	)
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewPeriodicReader(otelslog.NewMetricExporter(logger))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(mp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
		_ = mp.Shutdown(shutdownCtx)
	}
}

// parseLogLevel maps a LYRA_LOG_LEVEL string onto an slog level, defaulting
// to Info on empty / unrecognized input.
func parseLogLevel(s string) stdslog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return stdslog.LevelDebug
	case "warn", "warning":
		return stdslog.LevelWarn
	case "error":
		return stdslog.LevelError
	default:
		return stdslog.LevelInfo
	}
}
