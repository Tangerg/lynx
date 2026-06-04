package slog_test

import (
	"context"
	stdslog "log/slog"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Tangerg/lynx/otel/slog"
)

// TestMetricExporter_WritesOneRecordPerMetric drives a real MeterProvider
// through the exporter and asserts each instrument lands as a "metric"
// slog record carrying the instrument name — the Metrics leg of the dev
// triad, sharing the same slog stream as spans and logs.
func TestMetricExporter_WritesOneRecordPerMetric(t *testing.T) {
	cap := &captureHandler{}
	logger := stdslog.New(cap)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test")

	ctr, err := meter.Int64Counter("runs.started")
	if err != nil {
		t.Fatalf("counter: %v", err)
	}
	ctr.Add(context.Background(), 3, metric.WithAttributes(attribute.String("k", "v")))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	exp := slog.NewMetricExporter(logger)
	if err := exp.Export(context.Background(), &rm); err != nil {
		t.Fatalf("export: %v", err)
	}

	found := false
	for _, r := range cap.Records() {
		if r.Message != "metric" {
			continue
		}
		r.Attrs(func(a stdslog.Attr) bool {
			if a.Key == "metric" && a.Value.String() == "runs.started" {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Fatal("expected a metric record for runs.started")
	}
}
