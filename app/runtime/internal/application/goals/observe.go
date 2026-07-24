package goals

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// The autonomous loop is a request-detached background driver — without a span
// per turn and a disposition metric it runs blind. The loop's ctx keeps the
// starting request's trace values (taskgroup.Attach → context.WithoutCancel), so
// each goal.turn span nests under the goals.start trace (full-link). No-op until
// a TracerProvider / MeterProvider is installed.
const observeScope = "lynx/lyra/goal"

var driverTracer = otel.Tracer(observeScope)

// turnDisposition labels how one autonomous turn ended — the span attribute and
// metric dimension. dispContinue means the loop launches another turn; the other
// three are terminal. The zero value means the turn never completed and is not
// metered.
type turnDisposition string

const (
	dispContinue turnDisposition = "continue"
	dispComplete turnDisposition = "complete"
	dispBlocked  turnDisposition = "blocked"
	dispPaused   turnDisposition = "paused"
)

var loadGoalTurns = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(observeScope).Int64Counter("goal.turns",
		metric.WithDescription("Autonomous goal turns, by disposition (continue/complete/blocked/paused)."))
	return counter
})

func recordGoalTurn(ctx context.Context, disposition turnDisposition) {
	loadGoalTurns().Add(ctx, 1, metric.WithAttributes(attribute.String("goal.disposition", string(disposition))))
}

// recordSaveError attaches a goal-state persistence failure to the current turn
// span instead of dropping it. The save is best-effort (the boot reconcile is
// the durable safety net), but a silent drop would hide a store fault; the span
// keeps it visible without failing the loop.
func recordSaveError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
