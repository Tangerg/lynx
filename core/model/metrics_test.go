package model_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Tangerg/lynx/core/model"
)

// TestRecordOperationMetrics installs a ManualReader-backed
// MeterProvider, records one successful and one failed operation, then
// asserts the GenAI client metrics fire with the right values and tags.
// The package-level instruments are created against the global meter at
// init; SetMeterProvider delegates them to this provider.
func TestRecordOperationMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	dims := model.OperationMetrics{
		Operation:     "chat",
		System:        "openai",
		RequestModel:  "gpt-4o",
		ResponseModel: "gpt-4o",
	}
	model.RecordOperationMetrics(context.Background(), dims,
		&model.Usage{PromptTokens: 10, CompletionTokens: 5}, 1500*time.Millisecond, nil)
	model.RecordOperationMetrics(context.Background(), dims,
		nil, 250*time.Millisecond, errors.New("boom"))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}

	tokens := findHistogramInt64(t, &rm, model.MetricGenAIClientTokenUsage)
	duration := findHistogramFloat64(t, &rm, model.MetricGenAIClientOperationDuration)

	// Token usage: one input datapoint (10) and one output datapoint (5),
	// only from the successful record.
	if got := histInt64Sum(tokens, "gen_ai.token.type", "input"); got != 10 {
		t.Fatalf("input token sum = %d, want 10", got)
	}
	if got := histInt64Sum(tokens, "gen_ai.token.type", "output"); got != 5 {
		t.Fatalf("output token sum = %d, want 5", got)
	}

	// Duration: a success datapoint (no error.type) and an error
	// datapoint (error.type set). Sum across both ≈ 1.75s.
	var total float64
	var sawError bool
	for _, dp := range duration.DataPoints {
		total += dp.Sum
		if _, ok := dp.Attributes.Value(attribute.Key("error.type")); ok {
			sawError = true
		}
	}
	if total < 1.74 || total > 1.76 {
		t.Fatalf("duration sum = %v, want ≈1.75", total)
	}
	if !sawError {
		t.Fatal("expected an error.type-tagged duration datapoint")
	}

	// The provider/model tags must be present (low-cardinality dims).
	if got := histInt64Sum(tokens, "gen_ai.system", "openai"); got != 15 {
		t.Fatalf("openai-tagged token sum = %d, want 15 (10 input + 5 output)", got)
	}
}

func findHistogramInt64(t *testing.T, rm *metricdata.ResourceMetrics, name string) metricdata.Histogram[int64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				h, ok := m.Data.(metricdata.Histogram[int64])
				if !ok {
					t.Fatalf("%s is not an int64 histogram", name)
				}
				return h
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Histogram[int64]{}
}

func findHistogramFloat64(t *testing.T, rm *metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				h, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("%s is not a float64 histogram", name)
				}
				return h
			}
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Histogram[float64]{}
}

// histInt64Sum sums the histogram datapoints whose attribute key equals
// the wanted value.
func histInt64Sum(h metricdata.Histogram[int64], key, value string) int64 {
	var sum int64
	for _, dp := range h.DataPoints {
		if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == value {
			sum += dp.Sum
		}
	}
	return sum
}
