package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	agent "github.com/Tangerg/scope/agent"
	agentotel "github.com/Tangerg/scope/otel/agent"
)

func TestObserverTracesRealProcessStepAndEffectLifecycle(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observer, err := agentotel.NewObserver(agentotel.ObserverConfig{
		TracerProvider: provider, MeterProvider: meterProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	deployment := testDeployment(t)
	engine, err := agent.NewEngine(agent.EngineConfig{
		EventListeners: []agent.EventListener{observer},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(testInput{Value: "observed"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil || result.Status() != agent.StatusCompleted {
		t.Fatalf("result status = %s, error = %v", result.Status(), err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	spans := recorder.Ended()
	if len(spans) != 4 {
		for _, span := range spans {
			t.Logf("span %s parent=%s", span.Name(), span.Parent().SpanID())
		}
		t.Fatalf("ended spans = %d, want process + two steps + effect", len(spans))
	}
	process := spanByName(t, spans, "agent.process", 0)
	steps := spansByName(spans, "agent.step")
	effect := spanByName(t, spans, "agent.effect", 0)
	if len(steps) != 2 {
		t.Fatalf("step spans = %d, want 2", len(steps))
	}
	for _, span := range append(steps, effect) {
		if span.Parent().SpanID() != process.SpanContext().SpanID() {
			t.Fatalf("span %s parent = %s, want process %s", span.Name(), span.Parent().SpanID(), process.SpanContext().SpanID())
		}
		if span.EndTime().Before(span.StartTime()) {
			t.Fatalf("span %s has negative duration", span.Name())
		}
	}
	if got := stringAttribute(process.Attributes(), "agent.deployment.name"); got != "test.otel" {
		t.Fatalf("deployment name attribute = %q", got)
	}
	if got := stringAttribute(effect.Attributes(), "agent.effect.target"); got != "dispatcher" {
		t.Fatalf("effect target attribute = %q", got)
	}
	if got := stringAttribute(effect.Attributes(), "agent.effect.status"); got != "succeeded" {
		t.Fatalf("effect status attribute = %q", got)
	}
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	if got := int64Sum(t, metricByName(t, metrics, "agent.process.starts")); got != 1 {
		t.Fatalf("process starts = %d, want 1", got)
	}
	if got := int64Sum(t, metricByName(t, metrics, "agent.process.exits")); got != 1 {
		t.Fatalf("process exits = %d, want 1", got)
	}
	if got := histogramCount(t, metricByName(t, metrics, "agent.step.duration")); got != 2 {
		t.Fatalf("step duration observations = %d, want 2", got)
	}
	if got := histogramCount(t, metricByName(t, metrics, "agent.effect.duration")); got != 1 {
		t.Fatalf("effect duration observations = %d, want 1", got)
	}
}

func TestObserverRecordsStableProcessFailureAttribution(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observer, err := agentotel.NewObserver(agentotel.ObserverConfig{
		TracerProvider: provider, MeterProvider: meterProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	engine, err := agent.NewEngine(agent.EngineConfig{EventListeners: []agent.EventListener{observer}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(testInput{Value: "fail"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(t.Context(), testDeployment(t), input)
	if err != nil || result.Status() != agent.StatusFailed {
		t.Fatalf("result = %s, error = %v", result.Status(), err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}

	process := spanByName(t, recorder.Ended(), "agent.process", 0)
	if got := stringAttribute(process.Attributes(), "agent.failure.kind"); got != "execution" {
		t.Fatalf("failure kind = %q", got)
	}
	if got := stringAttribute(process.Attributes(), "agent.failure.code"); got != "test.otel.failed" {
		t.Fatalf("failure code = %q", got)
	}
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &metrics); err != nil {
		t.Fatal(err)
	}
	exits := metricByName(t, metrics, "agent.process.exits")
	assertSumAttribute(t, exits, "agent.failure.kind", "execution")
	assertSumAttribute(t, exits, "agent.failure.code", "test.otel.failed")
}

func TestObserverRejectsTypedNilTracerProvider(t *testing.T) {
	var provider *sdktrace.TracerProvider
	observer, err := agentotel.NewObserver(agentotel.ObserverConfig{TracerProvider: provider})
	if observer != nil || !errors.Is(err, agentotel.ErrInvalidObserverConfig) {
		t.Fatalf("observer = %v, error = %v", observer, err)
	}
}

func TestObserverIgnoresEventsAfterClose(t *testing.T) {
	var events []agent.Event
	engine, err := agent.NewEngine(agent.EngineConfig{EventListeners: []agent.EventListener{
		agent.EventListenerFunc(func(_ context.Context, event agent.Event) {
			events = append(events, event)
		}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(testInput{Value: "closed observer"})
	if err != nil {
		t.Fatal(err)
	}
	if result, runErr := engine.Run(context.Background(), testDeployment(t), input); runErr != nil || !result.Valid() {
		t.Fatalf("result = %#v, error = %v", result, runErr)
	}
	if closeErr := engine.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observer, err := agentotel.NewObserver(agentotel.ObserverConfig{MeterProvider: meterProvider})
	if err != nil {
		t.Fatal(err)
	}
	observer.Close()
	for _, event := range events {
		observer.OnEvent(context.Background(), event)
	}
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatal(err)
	}
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					if point.Value != 0 {
						t.Fatalf("metric %q recorded %d after Close", metric.Name, point.Value)
					}
				}
			case metricdata.Histogram[float64]:
				for _, point := range data.DataPoints {
					if point.Count != 0 {
						t.Fatalf("metric %q recorded %d samples after Close", metric.Name, point.Count)
					}
				}
			}
		}
	}
}

type testInput struct {
	Value string `json:"value"`
}

type testOutput struct {
	Value string `json:"value"`
}

type testDefinition struct {
	descriptor agent.Descriptor
}

func (t testDefinition) Descriptor() agent.Descriptor { return t.descriptor }

func (testDefinition) Start(input agent.Input) (agent.Execution, error) {
	value, err := input.Decode[testInput]()
	if err != nil {
		return nil, err
	}
	return &testExecution{Value: value.Value}, nil
}

func (testDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "test.otel" || state.SchemaVersion() != 1 {
		return nil, agent.ErrInvalidExecutionState
	}
	var execution testExecution
	if err := json.Unmarshal(state.Payload(), &execution); err != nil {
		return nil, err
	}
	return &execution, nil
}

type testExecution struct {
	Value string `json:"value"`
	Phase uint8  `json:"phase"`
}

func (t *testExecution) Step(context.Context, []agent.Signal) (agent.Transition, error) {
	if t.Phase == 0 && t.Value == "fail" {
		failure, err := agent.NewFailure(agent.FailureKindExecution, "test.otel.failed", "test failure")
		if err != nil {
			return agent.Transition{}, err
		}
		t.Phase = 2
		return agent.Fail(0, failure)
	}
	if t.Phase == 0 {
		effect, err := agent.NewDispatcherEffect(json.RawMessage(`{"operation":"observe"}`))
		if err != nil {
			return agent.Transition{}, err
		}
		t.Phase = 1
		return agent.Continue(0, effect)
	}
	output, err := agent.EncodeOutput(testOutput{Value: t.Value})
	if err != nil {
		return agent.Transition{}, err
	}
	t.Phase = 2
	return agent.Complete(1, output)
}

func (t *testExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(struct {
		Value string `json:"value"`
		Phase uint8  `json:"phase"`
	}{Value: t.Value, Phase: t.Phase})
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("test.otel", 1, payload)
}

type testDispatcher struct{}

func (testDispatcher) Dispatch(
	_ context.Context,
	request agent.EffectRequest,
	_ agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.NewSettlement(request.ID(), agent.SettlementStatusSucceeded, json.RawMessage(`{"ok":true}`))
}

func (testDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicySameIdentity
}

func testDeployment(t *testing.T) agent.Deployment {
	t.Helper()
	inputSchema, err := agent.SchemaFor[testInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := agent.SchemaFor[testOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "test.otel", Description: "Exercises the OpenTelemetry Event adapter.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: testDefinition{descriptor: descriptor}, Dispatcher: testDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("test-otel-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("test-otel-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func spansByName(spans []sdktrace.ReadOnlySpan, name string) []sdktrace.ReadOnlySpan {
	var matched []sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == name {
			matched = append(matched, span)
		}
	}
	return matched
}

func spanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string, index int) sdktrace.ReadOnlySpan {
	t.Helper()
	matched := spansByName(spans, name)
	if index >= len(matched) {
		t.Fatalf("span %q index %d is missing", name, index)
	}
	return matched[index]
}

func stringAttribute(attributes []attribute.KeyValue, key string) string {
	for _, value := range attributes {
		if string(value.Key) == key {
			return value.Value.AsString()
		}
	}
	return ""
}

func metricByName(t *testing.T, metrics metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, candidate := range scope.Metrics {
			if candidate.Name == name {
				return candidate
			}
		}
	}
	t.Fatalf("metric %q is missing", name)
	return metricdata.Metrics{}
}

func int64Sum(t *testing.T, metric metricdata.Metrics) int64 {
	t.Helper()
	data, ok := metric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data = %T, want int64 sum", metric.Name, metric.Data)
	}
	var total int64
	for _, point := range data.DataPoints {
		total += point.Value
	}
	return total
}

func assertSumAttribute(t *testing.T, metric metricdata.Metrics, key, want string) {
	t.Helper()
	data, ok := metric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data = %T, want int64 sum", metric.Name, metric.Data)
	}
	for _, point := range data.DataPoints {
		value, found := point.Attributes.Value(attribute.Key(key))
		if found && value.AsString() == want {
			return
		}
	}
	t.Fatalf("metric %q attribute %s = %q is missing", metric.Name, key, want)
}

func histogramCount(t *testing.T, metric metricdata.Metrics) uint64 {
	t.Helper()
	data, ok := metric.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("metric %q data = %T, want float64 histogram", metric.Name, metric.Data)
	}
	var total uint64
	for _, point := range data.DataPoints {
		total += point.Count
	}
	return total
}
