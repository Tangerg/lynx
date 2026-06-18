package model

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// GenAI client metric names per the OpenTelemetry GenAI semantic
// conventions. Emitting these standard names lets downstream collectors
// (Prometheus, Tempo, Honeycomb, …) aggregate token spend and latency
// without framework-specific wiring — the metric counterpart to the spans in
// each modality's tracing.go.
//
// https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-metrics/
const (
	MetricGenAIClientTokenUsage        = "gen_ai.client.token.usage"
	MetricGenAIClientOperationDuration = "gen_ai.client.operation.duration"
)

// Metric attribute keys. These are LOW-CARDINALITY dimensions only —
// every value must come from a bounded set (operation names, provider
// names, model ids, the fixed token-type / error-type enums). Never add
// a high-cardinality dimension here (request id, user id, raw prompt):
// metric tags multiply, and unbounded tags blow up the time-series
// count. High-cardinality detail belongs on spans, not metrics.
const (
	attrMetricOperation     = "gen_ai.operation.name"
	attrMetricSystem        = "gen_ai.system"
	attrMetricRequestModel  = "gen_ai.request.model"
	attrMetricResponseModel = "gen_ai.response.model"
	attrMetricTokenType     = "gen_ai.token.type"
	attrMetricErrorType     = "error.type"
)

// Token-type attribute values for [MetricGenAIClientTokenUsage].
const (
	tokenTypeInput  = "input"
	tokenTypeOutput = "output"
)

// genAIMeter is the package-level meter. Like the global tracer, the
// global meter delegates: instruments created here before a
// MeterProvider is installed forward to the real provider once
// otel.SetMeterProvider runs, and stay no-op (zero-cost) until then.
var genAIMeter = otel.Meter("lynx/gen_ai")

// Instruments are created once at package init. The global meter never
// errors for these valid name/unit pairs; a nil instrument (impossible
// here) would simply no-op on Record.
var (
	tokenUsageHistogram, _ = genAIMeter.Int64Histogram(
		MetricGenAIClientTokenUsage,
		metric.WithUnit("{token}"),
		metric.WithDescription("Number of input and output tokens used per GenAI request."),
	)
	operationDurationHistogram, _ = genAIMeter.Float64Histogram(
		MetricGenAIClientOperationDuration,
		metric.WithUnit("s"),
		metric.WithDescription("Duration of a GenAI client operation."),
	)
)

// OperationMetrics carries the low-cardinality dimensions shared by the
// GenAI client metrics for one model operation. Every field is a metric
// tag, so keep them bounded (see the attribute-key comment above).
type OperationMetrics struct {
	// Operation is the gen_ai.operation.name value ("chat", "embeddings").
	Operation string

	// System is the provider id (gen_ai.system), lowercased. Empty when
	// the model did not surface a provider.
	System string

	RequestModel string

	// ResponseModel is the model id the provider actually served. Empty
	// when unknown; falls back to RequestModel for the metric tag.
	ResponseModel string
}

// baseAttrs returns the dimension tags common to both metrics. The
// response model defaults to the request model when the provider did not
// echo one back, so the two metrics share a stable model tag.
func (d OperationMetrics) baseAttrs() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 4)
	if d.Operation != "" {
		attrs = append(attrs, attribute.String(attrMetricOperation, d.Operation))
	}
	if d.System != "" {
		attrs = append(attrs, attribute.String(attrMetricSystem, d.System))
	}
	if d.RequestModel != "" {
		attrs = append(attrs, attribute.String(attrMetricRequestModel, d.RequestModel))
	}
	responseModel := d.ResponseModel
	if responseModel == "" {
		responseModel = d.RequestModel
	}
	if responseModel != "" {
		attrs = append(attrs, attribute.String(attrMetricResponseModel, responseModel))
	}
	return attrs
}

// RecordOperationMetrics records the GenAI client token-usage and
// operation-duration metrics for one completed operation. It is the
// metric companion to the per-call span: call it once per operation,
// after the model returns.
//
// Duration is always recorded (success and failure, with an error.type
// tag on failure). Token usage is recorded only on success and only for
// the dimensions the provider surfaced — a nil usage or a zero count for
// a given direction is skipped. Until a MeterProvider is configured every
// Record is a no-op, so this is zero-cost by default.
func RecordOperationMetrics(ctx context.Context, dims OperationMetrics, usage *Usage, elapsed time.Duration, err error) {
	base := dims.baseAttrs()
	// withAttr returns base plus one extra tag, copied into a fresh
	// slice so the appends never alias base's backing array across the
	// three Record calls below.
	withAttr := func(extra attribute.KeyValue) []attribute.KeyValue {
		return append(append([]attribute.KeyValue(nil), base...), extra)
	}

	durationAttrs := base
	if errType := errorTypeName(err); errType != "" {
		durationAttrs = withAttr(attribute.String(attrMetricErrorType, errType))
	}
	operationDurationHistogram.Record(ctx, elapsed.Seconds(), metric.WithAttributes(durationAttrs...))

	if err != nil || usage == nil {
		return
	}
	if usage.PromptTokens > 0 {
		tokenUsageHistogram.Record(ctx, usage.PromptTokens,
			metric.WithAttributes(withAttr(attribute.String(attrMetricTokenType, tokenTypeInput))...))
	}
	if usage.CompletionTokens > 0 {
		tokenUsageHistogram.Record(ctx, usage.CompletionTokens,
			metric.WithAttributes(withAttr(attribute.String(attrMetricTokenType, tokenTypeOutput))...))
	}
}

// errorTypeName returns a bounded label for the error.type metric tag —
// the concrete Go type of the error (e.g. "*fmt.wrapError",
// "*tool.MaxIterationsError"). Error types are a bounded set, so this
// stays low-cardinality. Returns "" for a nil error.
func errorTypeName(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
