package runtime_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestMetrics_RecordedDuringRun installs a manual-reader MeterProvider, runs
// an agent, and confirms the runtime emitted the tick / action / plan / exit
// instruments. The runtime's instruments are created from the global
// (delegating) meter, so setting the provider here wires them to our reader.
func TestMetrics_RecordedDuringRun(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	a := agent.New("metered").
		Actions(agent.NewAction("count",
			func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, a)

	if _, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{}); err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	seen := collectedMetricNames(rm)
	for _, want := range []string{
		"lynx.agent.ticks",
		"lynx.agent.action.executions",
		"lynx.agent.plan.duration",
		"lynx.agent.process.exits",
	} {
		if !seen[want] {
			t.Errorf("metric %q not recorded; saw %v", want, seen)
		}
	}
}

func collectedMetricNames(rm metricdata.ResourceMetrics) map[string]bool {
	names := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names[m.Name] = true
		}
	}
	return names
}
