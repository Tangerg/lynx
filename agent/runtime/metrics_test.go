package runtime_test

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

var (
	runtimeMetricOnce   sync.Once
	runtimeMetricReader *sdkmetric.ManualReader
)

func installRuntimeMetricCapture() *sdkmetric.ManualReader {
	runtimeMetricOnce.Do(func() {
		runtimeMetricReader = sdkmetric.NewManualReader()
		otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(runtimeMetricReader)))
	})
	return runtimeMetricReader
}

// TestMetrics_RecordedDuringRun uses the package's process-lifetime
// MeterProvider and confirms the runtime emitted the tick / action / plan /
// exit instruments.
func TestMetrics_RecordedDuringRun(t *testing.T) {
	reader := installRuntimeMetricCapture()

	a := agent.New(agent.AgentConfig{Name: "metered", Actions: []agent.Action{agent.NewAction("count", func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "counted"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	if _, err := engine.Run(context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	seen := collectedMetricNames(rm)
	for _, want := range []string{
		"agent.ticks",
		"agent.action.executions",
		"agent.plan.duration",
		"agent.process.exits",
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
