package main

import (
	"context"
	stdslog "log/slog"
	"os"
	"strings"
	"time"

	slogbridge "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	otelslog "github.com/Tangerg/lynx/otel/slog"
)

// scopeName names the slog→OTel logs bridge's instrumentation scope.
const scopeName = "lyra"

// setupObservability wires all three OpenTelemetry signals — Traces,
// Metrics, Logs — through the global OTel providers onto one dev slog sink.
// It is the one place providers are set; everything else just uses the
// package-static otel.Tracer / otel.Meter / slog accessors, no injection.
//
//   - Traces:  TracerProvider, synchronous slog span exporter, AlwaysSample.
//   - Metrics: MeterProvider, PeriodicReader → slog metric exporter.
//   - Logs:    LoggerProvider → slog log exporter; slog.Default() is the
//              contrib otelslog bridge feeding that provider, so business
//              code calls slog.InfoContext and the records flow AS OTel log
//              records (trace_id / span_id filled natively from the active
//              span). Routing logs through OTel — rather than writing slog
//              directly — is what makes them backend-swappable: a production
//              build swaps every exporter for OTLP (→ Datadog / Cloud
//              Logging / ...) with zero business-code change.
//   - Context: W3C trace-context + baggage propagator, so a traceparent the
//              frontend sends extends into the backend (full-link tracing).
//
// Level comes from LYRA_LOG_LEVEL (debug|info|warn|error, default info) and
// is gated before the bridge (the bridge itself does no level filtering).
//
// Returns a shutdown func that flushes + tears down the providers; call it
// on process exit.
func setupObservability(serviceVersion string) func(context.Context) {
	level := parseLogLevel(os.Getenv("LYRA_LOG_LEVEL"))

	// The base logger is the actual stderr sink every signal renders into;
	// the three OTel exporters write here. Its level gates the rendered
	// output. slog.Default() (set below) is a DIFFERENT logger — the bridge —
	// so there's no loop: app logs go default→bridge→OTel→LogExporter→base.
	base := stdslog.New(stdslog.NewTextHandler(os.Stderr, &stdslog.HandlerOptions{Level: level}))

	res := resource.NewSchemaless(
		attribute.String("service.name", scopeName),
		attribute.String("service.version", serviceVersion),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(otelslog.NewSpanExporter(base)),
	)
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewPeriodicReader(otelslog.NewMetricExporter(base))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(mp)

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(otelslog.NewLogExporter(base))),
	)
	logglobal.SetLoggerProvider(lp)
	// slog.Default() = level gate → otelslog bridge → the LoggerProvider above.
	stdslog.SetDefault(stdslog.New(minLevelHandler{
		level: level,
		inner: slogbridge.NewHandler(scopeName, slogbridge.WithLoggerProvider(lp)),
	}))

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
		_ = mp.Shutdown(shutdownCtx)
		_ = lp.Shutdown(shutdownCtx)
	}
}

// minLevelHandler drops records below level before they reach the OTel logs
// bridge — the bridge does no level filtering, so without this gate
// LYRA_LOG_LEVEL would be ignored. It only adds the Enabled check; Handle /
// WithAttrs / WithGroup delegate straight through.
type minLevelHandler struct {
	level stdslog.Level
	inner stdslog.Handler
}

func (h minLevelHandler) Enabled(_ context.Context, l stdslog.Level) bool { return l >= h.level }
func (h minLevelHandler) Handle(ctx context.Context, r stdslog.Record) error {
	return h.inner.Handle(ctx, r)
}
func (h minLevelHandler) WithAttrs(a []stdslog.Attr) stdslog.Handler {
	return minLevelHandler{level: h.level, inner: h.inner.WithAttrs(a)}
}
func (h minLevelHandler) WithGroup(name string) stdslog.Handler {
	return minLevelHandler{level: h.level, inner: h.inner.WithGroup(name)}
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
