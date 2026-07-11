package turn

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Meter scope + metric names. These measure the lyra-runtime turn
// boundary, distinct from (not duplicating) the lower layers: the
// business turn duration spans the whole tool loop + interrupts +
// post-turn maintenance — broader than core's per-LLM-call gen_ai
// duration — and the interrupt counter has no analog below. No-op until
// a MeterProvider is installed.
const (
	meterName = "lynx/lyra/runtime"

	metricTurnDuration   = "run.duration"
	metricTurnInterrupts = "run.interrupts"
)

// turnMetrics holds the lazily-created instruments, built once via
// [loadTurnMetrics] so every turn reuses the same handles.
type turnMetrics struct {
	duration   metric.Float64Histogram
	interrupts metric.Int64Counter
}

var loadTurnMetrics = sync.OnceValue(newTurnMetrics)

func newTurnMetrics() *turnMetrics {
	m := otel.Meter(meterName)
	// Instrument-creation errors yield usable no-op instruments, so they're
	// safe to drop — recording stays a no-op rather than panicking on a
	// misconfigured provider.
	duration, _ := m.Float64Histogram(metricTurnDuration,
		metric.WithDescription("Turn wall-clock time, by outcome and model."),
		metric.WithUnit("ms"))
	interrupts, _ := m.Int64Counter(metricTurnInterrupts,
		metric.WithDescription("HITL interrupts a turn parked on, by kind."))
	return &turnMetrics{duration: duration, interrupts: interrupts}
}

func millis(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

// recordTurnDuration records one finished turn's wall-clock against the
// duration histogram, dimensioned by outcome + model (both low
// cardinality; the session / run ids stay on the span + logs).
func recordTurnDuration(ctx context.Context, reason TurnEndReason, model string, dur time.Duration) {
	loadTurnMetrics().duration.Record(ctx, millis(dur), metric.WithAttributes(
		attribute.String(attrRunOutcome, reason.String()),
		attribute.String(attrGenAIRequestModel, model),
	))
}

// recordInterruptMetric counts one HITL park, dimensioned by kind.
func recordInterruptMetric(ctx context.Context, kind string) {
	loadTurnMetrics().interrupts.Add(ctx, 1, metric.WithAttributes(
		attribute.String(attrRunInterruptKind, kind),
	))
}
