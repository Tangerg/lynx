package runtime

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Tangerg/lynx/agent/core"
)

// Metric names + the meter scope. Via the OTel metric API so it slots
// into the same pipeline as the runtime's tracing. When no MeterProvider is configured the global
// meter is a no-op, so these add zero overhead by default.
const (
	meterName = "lynx/agent"

	metricTicks     = "agent.ticks"
	metricActions   = "agent.action.executions"
	metricActionDur = "agent.action.duration"
	metricPlanDur   = "agent.plan.duration"
	metricRunExits  = "agent.process.exits"
)

// agentMetrics holds the lazily-created instruments. Built once via
// [loadMetrics] so repeated process runs reuse the same handles.
type agentMetrics struct {
	ticks          metric.Int64Counter
	actions        metric.Int64Counter
	actionDuration metric.Float64Histogram
	planDuration   metric.Float64Histogram
	exits          metric.Int64Counter
}

var loadMetrics = sync.OnceValue(newAgentMetrics)

func newAgentMetrics() *agentMetrics {
	meter := otel.Meter(meterName)

	// Instrument-creation errors yield usable no-op instruments, so the
	// errors are safe to drop — recording stays a no-op rather than
	// panicking on a misconfigured provider.
	ticks, _ := meter.Int64Counter(metricTicks,
		metric.WithDescription("OODA tick iterations, by agent."))
	actions, _ := meter.Int64Counter(metricActions,
		metric.WithDescription("Action executions, by agent and final status."))
	actionDuration, _ := meter.Float64Histogram(metricActionDur,
		metric.WithDescription("Action execution wall-clock time."),
		metric.WithUnit("ms"))
	planDuration, _ := meter.Float64Histogram(metricPlanDur,
		metric.WithDescription("Planner formulation wall-clock time."),
		metric.WithUnit("ms"))
	exits, _ := meter.Int64Counter(metricRunExits,
		metric.WithDescription("Run-loop exits, by agent and status (completed/failed/waiting/...)."))

	return &agentMetrics{
		ticks:          ticks,
		actions:        actions,
		actionDuration: actionDuration,
		planDuration:   planDuration,
		exits:          exits,
	}
}

func millis(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func (p *Process) agentAttr() attribute.KeyValue {
	return attribute.String(attrAgentName, p.agent().Name())
}

func (p *Process) recordTickMetric(ctx context.Context) {
	loadMetrics().ticks.Add(ctx, 1, metric.WithAttributes(p.agentAttr()))
}

func (p *Process) recordActionMetric(ctx context.Context, status core.ActionStatus, duration time.Duration) {
	attributes := metric.WithAttributes(
		p.agentAttr(),
		attribute.String(attrActionStatus, status.String()),
	)
	metrics := loadMetrics()
	metrics.actions.Add(ctx, 1, attributes)
	metrics.actionDuration.Record(ctx, millis(duration), attributes)
}

func (p *Process) recordPlanMetric(ctx context.Context, duration time.Duration) {
	loadMetrics().planDuration.Record(ctx, millis(duration), metric.WithAttributes(p.agentAttr()))
}

func (p *Process) recordRunExitMetric(ctx context.Context) {
	loadMetrics().exits.Add(ctx, 1, metric.WithAttributes(
		p.agentAttr(),
		attribute.String(attrProcessStatus, p.Status().String()),
	))
}
