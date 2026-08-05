package goals

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// The autonomous loop is a request-detached background driver — without a span
// per Run and a disposition metric it runs blind. The loop's ctx keeps the
// starting request's trace values (taskgroup.Attach → context.WithoutCancel), so
// each Goal-run span nests under the root Goal trace. No-op until
// a TracerProvider / MeterProvider is installed.
const observeScope = "lynx/lyra/goal"

var driverTracer = otel.Tracer(observeScope)

// runDisposition labels how one autonomous Run ended — the span attribute and
// metric dimension. dispContinue means the loop launches another Run; the other
// three are terminal. The zero value means the Run never completed and is not
// metered.
type runDisposition string

const (
	dispContinue runDisposition = "continue"
	dispComplete runDisposition = "complete"
	dispBlocked  runDisposition = "blocked"
	dispPaused   runDisposition = "paused"
)

var loadGoalRuns = sync.OnceValue(func() metric.Int64Counter {
	// A creation error yields a usable no-op counter, so it's safe to drop.
	counter, _ := otel.Meter(observeScope).Int64Counter("goal.runs",
		metric.WithDescription("Autonomous Goal Runs, by disposition (continue/complete/blocked/paused)."))
	return counter
})

func recordGoalRun(ctx context.Context, disposition runDisposition) {
	loadGoalRuns().Add(ctx, 1, metric.WithAttributes(attribute.String("goal.disposition", string(disposition))))
}
