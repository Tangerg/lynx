// Package otel instruments Lynx protocol capabilities with OpenTelemetry.
//
// Instrumentation lives outside Core: applications opt in by composing the
// returned middleware around a core/chat Model or Streamer.
package otel

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	apiotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/semconv/v1.41.0/genaiconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/chat"
)

const instrumentationName = "github.com/Tangerg/lynx/otel"

var (
	// ErrInvalidChatConfig reports a missing or malformed provider identity.
	ErrInvalidChatConfig = errors.New("otel: invalid chat config")
	// ErrNilChatStream reports a wrapped Streamer that returned a nil sequence.
	ErrNilChatStream = errors.New("otel: nil chat stream sequence")
)

// ChatConfig identifies the remote GenAI provider and optionally supplies
// providers scoped to this middleware. Provider is normalized to lowercase so
// span and metric dimensions remain stable. The global OpenTelemetry providers
// are used when TracerProvider or MeterProvider is nil.
type ChatConfig struct {
	Provider       string
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

// ChatMiddleware adds GenAI spans and metrics to synchronous and streaming
// chat capabilities. It is immutable after construction and safe for
// concurrent use.
type ChatMiddleware struct {
	provider string
	tracer   trace.Tracer
	duration genaiconv.ClientOperationDuration
	tokens   genaiconv.ClientTokenUsage
}

// NewChat constructs chat instrumentation. Provider is required at the
// composition root instead of being added to the Core Model contract.
func NewChat(config ChatConfig) (*ChatMiddleware, error) {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	if provider == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrInvalidChatConfig)
	}

	tracerProvider := config.TracerProvider
	if tracerProvider == nil {
		tracerProvider = apiotel.GetTracerProvider()
	}
	meterProvider := config.MeterProvider
	if meterProvider == nil {
		meterProvider = apiotel.GetMeterProvider()
	}

	meter := meterProvider.Meter(instrumentationName)
	duration, err := genaiconv.NewClientOperationDuration(meter)
	if err != nil {
		return nil, fmt.Errorf("%w: create duration histogram: %w", ErrInvalidChatConfig, err)
	}
	tokens, err := genaiconv.NewClientTokenUsage(meter)
	if err != nil {
		return nil, fmt.Errorf("%w: create token histogram: %w", ErrInvalidChatConfig, err)
	}

	return &ChatMiddleware{
		provider: provider,
		tracer:   tracerProvider.Tracer(instrumentationName),
		duration: duration,
		tokens:   tokens,
	}, nil
}

// Call is a [chat.CallMiddleware]. It preserves the wrapped model's response
// and error exactly; observation is a read-only side effect.
func (m *ChatMiddleware) Call(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		started := time.Now()
		ctx, span := m.start(ctx, request)
		response, err := next.Call(ctx, request)
		m.finish(ctx, span, request, response, err, time.Since(started))
		return response, err
	})
}

// Stream is a [chat.StreamMiddleware]. Instrumentation starts lazily when the
// caller iterates and ends synchronously on completion, provider failure, or
// early consumer stop. Invalid deltas are still forwarded unchanged; an
// accumulation problem is recorded as an event and never becomes a business
// error.
func (m *ChatMiddleware) Stream(next chat.Streamer) chat.Streamer {
	return chat.StreamerFunc(func(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			started := time.Now()
			spanCtx, span := m.start(ctx, request)
			var (
				accumulator chat.ResponseAccumulator
				streamErr   error
				firstToken  bool
				stopped     bool
			)
			defer func() {
				m.finish(spanCtx, span, request, accumulator.Response(), streamErr, time.Since(started))
			}()

			sequence := next.Stream(spanCtx, request)
			if sequence == nil {
				streamErr = ErrNilChatStream
				yield(nil, streamErr)
				return
			}
			sequence(func(chunk *chat.Response, err error) bool {
				if stopped {
					return false
				}
				if err != nil {
					streamErr = err
					stopped = true
					yield(chunk, err)
					return false
				}
				if !firstToken && hasGeneratedContent(chunk) {
					span.AddEvent("first_token_received")
					firstToken = true
				}
				if err := accumulator.Add(chunk); err != nil {
					span.AddEvent("gen_ai.stream.accumulation_error",
						trace.WithAttributes(semconv.ErrorTypeKey.String(errorType(err))),
					)
				}
				stopped = !yield(chunk, nil)
				return !stopped
			})
		}
	})
}

func (m *ChatMiddleware) start(
	ctx context.Context,
	request *chat.Request,
) (context.Context, trace.Span) {
	model := requestModel(request)
	name := "chat"
	if model != "" {
		name = "chat " + model
	}
	attrs := requestAttributes(request)
	attrs = append(attrs,
		semconv.GenAIOperationNameChat,
		semconv.GenAIProviderNameKey.String(m.provider),
	)
	return m.tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

func (m *ChatMiddleware) finish(
	ctx context.Context,
	span trace.Span,
	request *chat.Request,
	response *chat.Response,
	err error,
	elapsed time.Duration,
) {
	defer span.End()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetAttributes(responseAttributes(response)...)
	}
	m.recordMetrics(ctx, request, response, elapsed, err)
}

func (m *ChatMiddleware) recordMetrics(
	ctx context.Context,
	request *chat.Request,
	response *chat.Response,
	elapsed time.Duration,
	err error,
) {
	attrs := metricAttributes(request, response)
	if err != nil {
		attrs = append(attrs, semconv.ErrorTypeKey.String(errorType(err)))
	}
	m.duration.Record(ctx, elapsed.Seconds(),
		genaiconv.OperationNameChat,
		genaiconv.ProviderNameAttr(m.provider),
		attrs...,
	)
	if err != nil || response == nil {
		return
	}
	if response.Usage.InputTokens > 0 {
		m.tokens.Record(ctx, response.Usage.InputTokens,
			genaiconv.OperationNameChat,
			genaiconv.ProviderNameAttr(m.provider),
			genaiconv.TokenTypeInput,
			attrs...,
		)
	}
	if response.Usage.OutputTokens > 0 {
		m.tokens.Record(ctx, response.Usage.OutputTokens,
			genaiconv.OperationNameChat,
			genaiconv.ProviderNameAttr(m.provider),
			genaiconv.TokenTypeOutput,
			attrs...,
		)
	}
}

func requestAttributes(request *chat.Request) []attribute.KeyValue {
	if request == nil {
		return nil
	}
	options := request.Options
	attrs := make([]attribute.KeyValue, 0, 8)
	if options.Model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(options.Model))
	}
	if options.MaxTokens != nil {
		attrs = append(attrs, semconv.GenAIRequestMaxTokensKey.Int64(*options.MaxTokens))
	}
	if options.Temperature != nil {
		attrs = append(attrs, semconv.GenAIRequestTemperature(*options.Temperature))
	}
	if options.TopP != nil {
		attrs = append(attrs, semconv.GenAIRequestTopP(*options.TopP))
	}
	if options.TopK != nil {
		attrs = append(attrs, semconv.GenAIRequestTopKKey.Int64(*options.TopK))
	}
	if options.FrequencyPenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestFrequencyPenalty(*options.FrequencyPenalty))
	}
	if options.PresencePenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestPresencePenalty(*options.PresencePenalty))
	}
	if len(options.Stop) > 0 {
		attrs = append(attrs, semconv.GenAIRequestStopSequences(options.Stop...))
	}
	return attrs
}

func responseAttributes(response *chat.Response) []attribute.KeyValue {
	if response == nil {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, 5)
	if response.ID != "" {
		attrs = append(attrs, semconv.GenAIResponseID(response.ID))
	}
	if response.Model != "" {
		attrs = append(attrs, semconv.GenAIResponseModel(response.Model))
	}
	finishReasons := make([]string, 0, len(response.Choices))
	for i := range response.Choices {
		if reason := response.Choices[i].FinishReason; reason != "" {
			finishReasons = append(finishReasons, reason.String())
		}
	}
	if len(finishReasons) > 0 {
		attrs = append(attrs, semconv.GenAIResponseFinishReasons(finishReasons...))
	}
	if response.Usage.InputTokens > 0 {
		attrs = append(attrs, semconv.GenAIUsageInputTokensKey.Int64(response.Usage.InputTokens))
	}
	if response.Usage.OutputTokens > 0 {
		attrs = append(attrs, semconv.GenAIUsageOutputTokensKey.Int64(response.Usage.OutputTokens))
	}
	return attrs
}

func metricAttributes(request *chat.Request, response *chat.Response) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	if model := requestModel(request); model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(model))
	}
	responseModel := ""
	if response != nil {
		responseModel = response.Model
	}
	if responseModel == "" {
		responseModel = requestModel(request)
	}
	if responseModel != "" {
		attrs = append(attrs, semconv.GenAIResponseModel(responseModel))
	}
	return attrs
}

func requestModel(request *chat.Request) string {
	if request == nil {
		return ""
	}
	return request.Options.Model
}

func hasGeneratedContent(response *chat.Response) bool {
	if response == nil {
		return false
	}
	for i := range response.Choices {
		message := response.Choices[i].Message
		if message != nil && len(message.Parts) > 0 {
			return true
		}
	}
	return false
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
