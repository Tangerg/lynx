package embedding

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model"
)

// embedTracer is the package-level tracer for embedding client span
// emission. Tracer name follows the gen_ai.operation convention.
//
// Calls into a not-yet-configured TracerProvider are no-op, so spans
// emitted here are zero-cost by default — see doc/OBSERVABILITY.md §5.
var embedTracer = otel.Tracer("lynx/gen_ai/embeddings")

// OpenTelemetry GenAI semconv attribute keys for embedding spans —
// only the subset the embedding operation populates. The set is a
// strict subset of the chat keys.
const (
	attrEmbedGenAISystem        = "gen_ai.system"
	attrEmbedGenAIOperationName = "gen_ai.operation.name"
	attrEmbedGenAIRequestModel  = "gen_ai.request.model"
	attrEmbedGenAIResponseModel = "gen_ai.response.model"
	// gen_ai.usage.input_tokens is the canonical "tokens consumed by
	// the embedded input" attribute per the OTel GenAI registry.
	attrEmbedGenAIUsageInputTokens = "gen_ai.usage.input_tokens"
	// lynx.embeddings.input.count is the lynx-specific extension that
	// records the batch size (number of texts). No GenAI attribute
	// covers this today.
	attrEmbedInputCount = "embeddings.input.count"
)

// startEmbeddingSpan opens one span for an embedding call following
// the OpenTelemetry GenAI semconv. Span name uses the canonical
// `embeddings <model>` shape (e.g. `embeddings text-embedding-3-small`);
// when the model id is empty the span name is just `embeddings`.
//
// Span kind is Client.
func startEmbeddingSpan(ctx context.Context, model Model, req *Request) (context.Context, trace.Span) {
	modelID := ""
	if req != nil && req.Options != nil {
		modelID = req.Options.Model
	}
	name := "embeddings"
	if modelID != "" {
		name = "embeddings " + modelID
	}

	attrs := make([]attribute.KeyValue, 0, 4)
	attrs = append(attrs, attribute.String(attrEmbedGenAIOperationName, "embeddings"))
	if model != nil {
		if provider := model.Metadata().Provider; provider != "" {
			attrs = append(attrs, attribute.String(attrEmbedGenAISystem, strings.ToLower(provider)))
		}
	}
	if modelID != "" {
		attrs = append(attrs, attribute.String(attrEmbedGenAIRequestModel, modelID))
	}
	if req != nil {
		attrs = append(attrs, attribute.Int(attrEmbedInputCount, len(req.Texts)))
	}

	return embedTracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// finishEmbeddingSpan records response-side attributes (resolved
// model id, input token usage) and ends the span.
func finishEmbeddingSpan(span trace.Span, resp *Response, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return
	}
	if resp != nil && resp.Metadata != nil {
		attrs := make([]attribute.KeyValue, 0, 2)
		if resp.Metadata.Model != "" {
			attrs = append(attrs, attribute.String(attrEmbedGenAIResponseModel, resp.Metadata.Model))
		}
		if resp.Metadata.Usage != nil {
			attrs = append(attrs, attribute.Int64(attrEmbedGenAIUsageInputTokens, resp.Metadata.Usage.PromptTokens))
		}
		if len(attrs) > 0 {
			span.SetAttributes(attrs...)
		}
	}
	span.End()
}

// recordEmbeddingMetrics emits the GenAI client metrics (input token
// usage + operation duration) for one embedding call. Call it once per
// call, passing the start time captured before [startEmbeddingSpan].
// Embeddings produce no completion tokens, so only the input dimension
// of [model.MetricGenAIClientTokenUsage] is recorded. No-op until a
// MeterProvider is configured.
func recordEmbeddingMetrics(ctx context.Context, m Model, req *Request, resp *Response, err error, start time.Time) {
	dims := model.OperationMetrics{Operation: "embeddings"}
	if m != nil {
		dims.System = strings.ToLower(m.Metadata().Provider)
	}
	if req != nil && req.Options != nil {
		dims.RequestModel = req.Options.Model
	}
	var usage *model.Usage
	if resp != nil && resp.Metadata != nil {
		dims.ResponseModel = resp.Metadata.Model
		usage = resp.Metadata.Usage
	}
	model.RecordOperationMetrics(ctx, dims, usage, time.Since(start), err)
}
