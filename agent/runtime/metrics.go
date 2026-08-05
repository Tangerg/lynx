package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/Tangerg/lynx/agent/core"
)

// Runtime metrics use the OTel pipeline shared with tracing. Without a
// MeterProvider the global meter is a no-op.
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

var loadMetrics = sync.OnceValue(func() *agentMetrics {
	return newAgentMetrics(otel.Meter(meterName))
})

func newAgentMetrics(meter metric.Meter) *agentMetrics {
	return &agentMetrics{
		ticks: int64Counter(meter, metricTicks,
			metric.WithDescription("OODA tick iterations, by agent.")),
		actions: int64Counter(meter, metricActions,
			metric.WithDescription("Action executions, by agent and final status.")),
		actionDuration: float64Histogram(meter, metricActionDur,
			metric.WithDescription("Action execution wall-clock time."),
			metric.WithUnit("ms")),
		planDuration: float64Histogram(meter, metricPlanDur,
			metric.WithDescription("Planner formulation wall-clock time."),
			metric.WithUnit("ms")),
		exits: int64Counter(meter, metricRunExits,
			metric.WithDescription("Run-loop exits, by agent and status (completed/failed/waiting/...).")),
	}
}

// Metrics are observational: a provider that rejects an instrument must not
// turn a valid process run into a panic. OTel's error handler keeps the failure
// visible while the typed no-op preserves the execution path.
func int64Counter(meter metric.Meter, name string, options ...metric.Int64CounterOption) metric.Int64Counter {
	instrument, err := meter.Int64Counter(name, options...)
	if err == nil && !valueIsNil(instrument) {
		return instrument
	}
	if err == nil {
		err = errors.New("provider returned a nil counter")
	}
	otel.Handle(fmt.Errorf("agent runtime: create metric %q: %w", name, err))
	return noop.Int64Counter{}
}

func float64Histogram(meter metric.Meter, name string, options ...metric.Float64HistogramOption) metric.Float64Histogram {
	instrument, err := meter.Float64Histogram(name, options...)
	if err == nil && !valueIsNil(instrument) {
		return instrument
	}
	if err == nil {
		err = errors.New("provider returned a nil histogram")
	}
	otel.Handle(fmt.Errorf("agent runtime: create metric %q: %w", name, err))
	return noop.Float64Histogram{}
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
