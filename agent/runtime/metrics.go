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

// Metric names + the meter scope. Mirrors embabel's Micrometer counters /
// timers, but via the OTel metric API so it slots into the same pipeline as
// the runtime's tracing. When no MeterProvider is configured the global
// meter is a no-op, so these add zero overhead by default.
const (
	meterName = "lynx/agent"

	metricTicks      = "agent.ticks"
	metricActions    = "agent.action.executions"
	metricActionDur  = "agent.action.duration"
	metricPlanDur    = "agent.plan.duration"
	metricRunExits   = "agent.process.exits"
	attrProcessState = "agent.process.status"
)

// agentMetrics holds the lazily-created instruments. Built once via
// [loadMetrics] so repeated process runs reuse the same handles.
type agentMetrics struct {
	ticks     metric.Int64Counter
	actions   metric.Int64Counter
	actionDur metric.Float64Histogram
	planDur   metric.Float64Histogram
	exits     metric.Int64Counter
}

var loadMetrics = sync.OnceValue(newAgentMetrics)

func newAgentMetrics() *agentMetrics {
	m := otel.Meter(meterName)

	// Instrument-creation errors yield usable no-op instruments, so the
	// errors are safe to drop — recording stays a no-op rather than
	// panicking on a misconfigured provider.
	ticks, _ := m.Int64Counter(metricTicks,
		metric.WithDescription("OODA tick iterations, by agent."))
	actions, _ := m.Int64Counter(metricActions,
		metric.WithDescription("Action executions, by agent and final status."))
	actionDur, _ := m.Float64Histogram(metricActionDur,
		metric.WithDescription("Action execution wall-clock time."),
		metric.WithUnit("ms"))
	planDur, _ := m.Float64Histogram(metricPlanDur,
		metric.WithDescription("Planner formulation wall-clock time."),
		metric.WithUnit("ms"))
	exits, _ := m.Int64Counter(metricRunExits,
		metric.WithDescription("Run-loop exits, by agent and status (completed/failed/waiting/...)."))

	return &agentMetrics{
		ticks:     ticks,
		actions:   actions,
		actionDur: actionDur,
		planDur:   planDur,
		exits:     exits,
	}
}

// millis renders a duration as fractional milliseconds for histograms.
func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func (p *AgentProcess) agentAttr() attribute.KeyValue {
	return attribute.String(attrAgentName, p.agent.Name)
}

func (p *AgentProcess) recordTickMetric(ctx context.Context) {
	loadMetrics().ticks.Add(ctx, 1, metric.WithAttributes(p.agentAttr()))
}

func (p *AgentProcess) recordActionMetric(ctx context.Context, status core.ActionStatus, dur time.Duration) {
	attrs := metric.WithAttributes(
		p.agentAttr(),
		attribute.String(attrActionStatus, status.String()),
	)
	m := loadMetrics()
	m.actions.Add(ctx, 1, attrs)
	m.actionDur.Record(ctx, millis(dur), attrs)
}

func (p *AgentProcess) recordPlanMetric(ctx context.Context, dur time.Duration) {
	loadMetrics().planDur.Record(ctx, millis(dur), metric.WithAttributes(p.agentAttr()))
}

func (p *AgentProcess) recordRunExitMetric(ctx context.Context) {
	loadMetrics().exits.Add(ctx, 1, metric.WithAttributes(
		p.agentAttr(),
		attribute.String(attrProcessState, p.Status().String()),
	))
}
