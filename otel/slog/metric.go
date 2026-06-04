package slog

import (
	"context"
	"fmt"
	stdslog "log/slog"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MetricExporter writes OpenTelemetry metric data to a log/slog logger —
// the Metrics leg of the dev observability triad, a sibling of [Exporter]
// (spans) so all three signals share one slog stream.
//
// Install it on a MeterProvider via a PeriodicReader:
//
//	reader := sdkmetric.NewPeriodicReader(slog.NewMetricExporter(logger))
//	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
//	otel.SetMeterProvider(mp)
//
// Each metric becomes one slog record carrying the instrument name, unit,
// scope, and a compact rendering of its data points. Like [Exporter] this
// is for local visibility; production should use an OTLP metric exporter.
type MetricExporter struct {
	logger *stdslog.Logger
}

// NewMetricExporter returns a metric exporter writing to logger; a nil
// logger defaults to stdslog.Default().
func NewMetricExporter(logger *stdslog.Logger) *MetricExporter {
	if logger == nil {
		logger = stdslog.Default()
	}
	return &MetricExporter{logger: logger}
}

// Temporality / Aggregation defer to the SDK defaults — this is a passive
// dev sink with no opinion on accumulation semantics.
func (e *MetricExporter) Temporality(k sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(k)
}

func (e *MetricExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}

// Export writes one slog record per metric. Always returns nil: a dev sink
// must never fail a collection cycle (mirrors [Exporter.ExportSpans]).
func (e *MetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			attrs := []stdslog.Attr{
				stdslog.String("metric", m.Name),
				stdslog.String("scope", sm.Scope.Name),
			}
			if m.Unit != "" {
				attrs = append(attrs, stdslog.String("unit", m.Unit))
			}
			attrs = append(attrs, stdslog.String("value", summarize(m.Data)))
			e.logger.LogAttrs(ctx, stdslog.LevelInfo, "metric", attrs...)
		}
	}
	return nil
}

func (e *MetricExporter) ForceFlush(ctx context.Context) error { return nil }
func (e *MetricExporter) Shutdown(ctx context.Context) error   { return nil }

// summarize renders a metric's aggregation as a compact human string for
// the dev log line — point count plus per-point value/sum/count, enough to
// eyeball a counter or histogram without a full backend.
func summarize(data metricdata.Aggregation) string {
	switch d := data.(type) {
	case metricdata.Sum[int64]:
		return sumStr(d.DataPoints)
	case metricdata.Sum[float64]:
		return sumStr(d.DataPoints)
	case metricdata.Gauge[int64]:
		return gaugeStr(d.DataPoints)
	case metricdata.Gauge[float64]:
		return gaugeStr(d.DataPoints)
	case metricdata.Histogram[int64]:
		return histStr(d.DataPoints)
	case metricdata.Histogram[float64]:
		return histStr(d.DataPoints)
	default:
		return fmt.Sprintf("%T", data)
	}
}

func sumStr[N int64 | float64](pts []metricdata.DataPoint[N]) string {
	var total N
	for _, p := range pts {
		total += p.Value
	}
	return fmt.Sprintf("sum=%v points=%d", total, len(pts))
}

func gaugeStr[N int64 | float64](pts []metricdata.DataPoint[N]) string {
	if len(pts) == 0 {
		return "gauge=<none>"
	}
	return fmt.Sprintf("gauge=%v points=%d", pts[len(pts)-1].Value, len(pts))
}

func histStr[N int64 | float64](pts []metricdata.HistogramDataPoint[N]) string {
	var count uint64
	var sum N
	for _, p := range pts {
		count += p.Count
		sum += p.Sum
	}
	return fmt.Sprintf("hist count=%d sum=%v points=%d", count, sum, len(pts))
}
