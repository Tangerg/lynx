package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model"
)

// chatTracer is the package-level tracer for chat client span
// emission. The OTel global accessor returns a delegating tracer, so
// later `otel.SetTracerProvider` calls in the user's program take
// effect on spans started here without re-wiring. When no provider is
// configured (the default), Start / End / SetAttributes compile down
// to no-op — see doc/OBSERVABILITY.md §5.
//
// Tracer name follows the convention from the OTel GenAI spec:
// instrumentation libraries SHOULD name their tracer after the
// gen_ai.operation namespace they instrument.
var chatTracer = otel.Tracer("lynx/gen_ai/chat")

type haltError interface {
	error
	Abort() bool
}

// isHITLInterrupt reports whether err is a HITL interrupt — a halt-style error
// whose Abort() is false (suspension for human input, not a fatal abort). The
// chat client span uses it to treat an interrupt as normal control flow rather
// than a failure, so production error-rate alerts don't fire on every approval.
func isHITLInterrupt(err error) bool {
	h, ok := errors.AsType[haltError](err)
	return ok && !h.Abort()
}

// OpenTelemetry GenAI semantic-convention attribute keys. The values
// these spans carry MUST match the spec under
// https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/
// so downstream collectors (Tempo, Jaeger, Honeycomb, …) and any
// auto-instrumentation hook (go.opentelemetry.io/auto/sdk) can
// recognize them without framework-specific wiring.
const (
	attrGenAISystem                = "gen_ai.system"
	attrGenAIOperationName         = "gen_ai.operation.name"
	attrGenAIRequestModel          = "gen_ai.request.model"
	attrGenAIRequestMaxTokens      = "gen_ai.request.max_tokens"
	attrGenAIRequestTemperature    = "gen_ai.request.temperature"
	attrGenAIRequestTopP           = "gen_ai.request.top_p"
	attrGenAIRequestTopK           = "gen_ai.request.top_k"
	attrGenAIRequestFrequencyPen   = "gen_ai.request.frequency_penalty"
	attrGenAIRequestPresencePen    = "gen_ai.request.presence_penalty"
	attrGenAIRequestStopSequences  = "gen_ai.request.stop_sequences"
	attrGenAIResponseID            = "gen_ai.response.id"
	attrGenAIResponseModel         = "gen_ai.response.model"
	attrGenAIResponseFinishReasons = "gen_ai.response.finish_reasons"
	attrGenAIUsageInputTokens      = "gen_ai.usage.input_tokens"
	attrGenAIUsageOutputTokens     = "gen_ai.usage.output_tokens"
)

// startChatSpan opens one span for a chat operation following the
// OpenTelemetry GenAI semconv. Span name uses the canonical
// `<operation> <model>` shape (e.g. `chat gpt-4o-mini`); when the
// model id is empty the span name falls back to the operation alone.
//
// Span kind is Client — the host process is invoking a remote LLM.
//
// The returned context carries the new span so any nested operations
// (tool calls, retrieval, sub-handlers) attach as children
// automatically via the normal otel.SpanFromContext path.
func startChatSpan(ctx context.Context, model Model, req *Request, operation string) (context.Context, trace.Span) {
	modelID := ""
	if req != nil && req.Options != nil {
		modelID = req.Options.Model
	}
	name := operation
	if modelID != "" {
		name = operation + " " + modelID
	}

	attrs := make([]attribute.KeyValue, 0, 10)
	attrs = append(attrs, attribute.String(attrGenAIOperationName, operation))
	if model != nil {
		if provider := model.Metadata().Provider; provider != "" {
			attrs = append(attrs, attribute.String(attrGenAISystem, strings.ToLower(provider)))
		}
	}
	if modelID != "" {
		attrs = append(attrs, attribute.String(attrGenAIRequestModel, modelID))
	}
	if req != nil && req.Options != nil {
		opts := req.Options
		if opts.MaxTokens != nil {
			attrs = append(attrs, attribute.Int64(attrGenAIRequestMaxTokens, *opts.MaxTokens))
		}
		if opts.Temperature != nil {
			attrs = append(attrs, attribute.Float64(attrGenAIRequestTemperature, *opts.Temperature))
		}
		if opts.TopP != nil {
			attrs = append(attrs, attribute.Float64(attrGenAIRequestTopP, *opts.TopP))
		}
		if opts.TopK != nil {
			attrs = append(attrs, attribute.Int64(attrGenAIRequestTopK, *opts.TopK))
		}
		if opts.FrequencyPenalty != nil {
			attrs = append(attrs, attribute.Float64(attrGenAIRequestFrequencyPen, *opts.FrequencyPenalty))
		}
		if opts.PresencePenalty != nil {
			attrs = append(attrs, attribute.Float64(attrGenAIRequestPresencePen, *opts.PresencePenalty))
		}
		if len(opts.Stop) > 0 {
			attrs = append(attrs, attribute.StringSlice(attrGenAIRequestStopSequences, opts.Stop))
		}
	}

	return chatTracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// finishChatSpan records the response-side attributes (model id,
// response id, finish reason, usage) and ends the span. A non-nil
// err sets the span status to Error and emits a stack-friendly
// RecordError event before ending.
func finishChatSpan(span trace.Span, resp *Response, err error) {
	if err != nil {
		if isHITLInterrupt(err) {
			// A HITL interrupt is normal control flow (the run paused for
			// human input), NOT a failure — record it as an event but leave
			// the span status unset so production error-rate alerts don't
			// fire on every approval.
			span.AddEvent("tool_loop.interrupted")
			span.End()
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return
	}
	if resp == nil {
		span.End()
		return
	}

	attrs := make([]attribute.KeyValue, 0, 5)
	if meta := resp.Metadata; meta != nil {
		if meta.ID != "" {
			attrs = append(attrs, attribute.String(attrGenAIResponseID, meta.ID))
		}
		if meta.Model != "" {
			attrs = append(attrs, attribute.String(attrGenAIResponseModel, meta.Model))
		}
		if meta.Usage != nil {
			attrs = append(attrs,
				attribute.Int64(attrGenAIUsageInputTokens, meta.Usage.PromptTokens),
				attribute.Int64(attrGenAIUsageOutputTokens, meta.Usage.CompletionTokens),
			)
		}
	}
	if r := resp.Result; r != nil && r.Metadata != nil && r.Metadata.FinishReason != "" {
		attrs = append(attrs,
			attribute.StringSlice(attrGenAIResponseFinishReasons, []string{string(r.Metadata.FinishReason)}),
		)
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	span.End()
}

// recordChatMetrics emits the GenAI client metrics (token usage +
// operation duration) for one chat operation. It is the metric companion
// to [finishChatSpan]: call it once per call/stream, passing the start
// time captured before [startChatSpan]. No-op until a MeterProvider is
// configured.
func recordChatMetrics(ctx context.Context, m Model, req *Request, resp *Response, err error, start time.Time) {
	dims := model.OperationMetrics{Operation: "chat"}
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
