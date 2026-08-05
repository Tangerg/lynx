package runtime

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
)

type invalidMeter struct {
	metric.Meter
	err error
}

func (m invalidMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, m.err
}

func (m invalidMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, m.err
}

func TestAgentMetricsFallbackToNoopInstruments(t *testing.T) {
	for _, test := range []struct {
		name  string
		meter metric.Meter
	}{
		{name: "provider error", meter: invalidMeter{err: errors.New("rejected")}},
		{name: "nil instrument", meter: invalidMeter{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			metrics := newAgentMetrics(test.meter)
			ctx := t.Context()
			metrics.ticks.Add(ctx, 1)
			metrics.actions.Add(ctx, 1)
			metrics.actionDuration.Record(ctx, 1)
			metrics.planDuration.Record(ctx, 1)
			metrics.exits.Add(ctx, 1)
		})
	}
}
