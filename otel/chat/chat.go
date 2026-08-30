// Package chat instruments Core chat capabilities with OpenTelemetry.
package chat

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

	"github.com/samber/lo"

	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	instrumentationName            = "github.com/Tangerg/scope/otel/chat"
	chatOperationName              = "chat"
	firstTokenReceivedEvent        = "first_token_received"
	timeToFirstTokenMetric         = "gen_ai.client.time_to_first_token"
	timeToFirstTokenDescription    = "Time to the first generated content in a streaming response."
	timeToFirstTokenUnit           = "s"
	streamAccumulationFailureEvent = "gen_ai.stream.accumulation_error"
	errorTypeContextCanceled       = "context.canceled"
	errorTypeDeadlineExceeded      = "context.deadline_exceeded"
	errorTypeInvalidRequest        = "chat.invalid_request"
	errorTypeInvalidResponse       = "chat.invalid_response"
	errorTypeInvalidMessage        = "chat.invalid_message"
	errorTypeInvalidPart           = "chat.invalid_part"
	errorTypeInvalidToolCall       = "chat.invalid_tool_call"
	errorTypeInvalidToolResult     = "chat.invalid_tool_result"
	errorTypeInvalidToolDefinition = "chat.invalid_tool_definition"
	errorTypeInvalidOutputFormat   = "chat.invalid_output_format"
	errorTypeInvalidOptions        = "chat.invalid_options"
	errorTypeInvalidUsage          = "chat.invalid_usage"
	errorTypeNilStream             = "otel.chat.nil_stream"
)

var (
	ErrInvalidConfig = errors.New("otel/chat: invalid config")
	ErrNilStream     = errors.New("otel/chat: nil stream sequence")
)

// MiddlewareConfig identifies the remote GenAI provider and optionally supplies
// providers scoped to this middleware. Provider is normalized to lowercase so
// span and metric dimensions remain stable. The global OpenTelemetry providers
// are used when TracerProvider or MeterProvider is nil.
type MiddlewareConfig struct {
	Provider       string
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

func (m MiddlewareConfig) Validate() error {
	if strings.TrimSpace(m.Provider) == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidConfig)
	}
	return nil
}

// Middleware adds GenAI spans and metrics to synchronous and streaming
// chat capabilities. It is immutable after construction and safe for
// concurrent use.
type Middleware struct {
	provider           string
	tracer             trace.Tracer
	duration           genaiconv.ClientOperationDuration
	tokens             genaiconv.ClientTokenUsage
	firstTokenDuration metric.Float64Histogram
}

func NewMiddleware(config MiddlewareConfig) (Middleware, error) {
	if err := config.Validate(); err != nil {
		return Middleware{}, err
	}
	provider := strings.ToLower(strings.TrimSpace(config.Provider))

	tracerProvider := config.TracerProvider
	if lo.IsNil(tracerProvider) {
		tracerProvider = apiotel.GetTracerProvider()
	}
	meterProvider := config.MeterProvider
	if lo.IsNil(meterProvider) {
		meterProvider = apiotel.GetMeterProvider()
	}

	meter := meterProvider.Meter(instrumentationName)
	duration, err := genaiconv.NewClientOperationDuration(meter)
	if err != nil {
		return Middleware{}, fmt.Errorf("%w: create duration histogram: %w", ErrInvalidConfig, err)
	}
	tokens, err := genaiconv.NewClientTokenUsage(meter)
	if err != nil {
		return Middleware{}, fmt.Errorf("%w: create token histogram: %w", ErrInvalidConfig, err)
	}
	firstTokenDuration, err := meter.Float64Histogram(
		timeToFirstTokenMetric,
		metric.WithDescription(timeToFirstTokenDescription),
		metric.WithUnit(timeToFirstTokenUnit),
	)
	if err != nil {
		return Middleware{}, fmt.Errorf("%w: create first-token histogram: %w", ErrInvalidConfig, err)
	}

	return Middleware{
		provider:           provider,
		tracer:             tracerProvider.Tracer(instrumentationName),
		duration:           duration,
		tokens:             tokens,
		firstTokenDuration: firstTokenDuration,
	}, nil
}

// Call is a [corechat.CallMiddleware]. It preserves the wrapped model's response
// and error exactly; observation is a read-only side effect.
func (m Middleware) Call(next corechat.Model) corechat.Model {
	if lo.IsNil(next) {
		return nil
	}
	return corechat.ModelFunc(func(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
		started := time.Now()
		ctx, span := m.start(ctx, request)
		response, err := next.Call(ctx, request)
		m.finish(ctx, span, request, response, err, time.Since(started))
		return response, err
	})
}

// Stream is a [corechat.StreamMiddleware]. Instrumentation starts lazily when the
// caller iterates and ends synchronously on completion, provider failure, or
// early consumer stop. Invalid deltas are still forwarded unchanged; an
// accumulation problem is recorded as an event and never becomes a business
// error.
func (m Middleware) Stream(next corechat.Streamer) corechat.Streamer {
	if lo.IsNil(next) {
		return nil
	}
	return corechat.StreamerFunc(func(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
		return func(yield func(*corechat.Response, error) bool) {
			started := time.Now()
			spanCtx, span := m.start(ctx, request)
			var (
				accumulator corechat.ResponseAccumulator
				streamErr   error
				firstToken  bool
				stopped     bool
			)
			defer func() {
				m.finish(spanCtx, span, request, accumulator.Response(), streamErr, time.Since(started))
			}()

			sequence := next.Stream(spanCtx, request)
			if sequence == nil {
				streamErr = ErrNilStream
				yield(nil, streamErr)
				return
			}
			sequence(func(chunk *corechat.Response, err error) bool {
				if stopped {
					return false
				}
				if !firstToken && hasGeneratedContent(chunk) {
					span.AddEvent(firstTokenReceivedEvent)
					m.recordTimeToFirstToken(spanCtx, request, chunk, time.Since(started))
					firstToken = true
				}
				if chunk != nil {
					if accumulationErr := accumulator.Add(chunk); accumulationErr != nil {
						span.AddEvent(streamAccumulationFailureEvent,
							trace.WithAttributes(errorTypeAttribute(accumulationErr)),
						)
					}
				}
				if err != nil {
					streamErr = err
					stopped = true
					yield(chunk, err)
					return false
				}
				stopped = !yield(chunk, nil)
				return !stopped
			})
		}
	})
}

func (m Middleware) recordTimeToFirstToken(
	ctx context.Context,
	request *corechat.Request,
	response *corechat.Response,
	elapsed time.Duration,
) {
	attributes := metricAttributes(request, response)
	attributes = append(attributes,
		semconv.GenAIOperationNameChat,
		semconv.GenAIProviderNameKey.String(m.provider),
	)
	m.firstTokenDuration.Record(
		ctx,
		elapsed.Seconds(),
		metric.WithAttributes(attributes...),
	)
}

func (m Middleware) start(
	ctx context.Context,
	request *corechat.Request,
) (context.Context, trace.Span) {
	model := requestModel(request)
	name := chatOperationName
	if model != "" {
		name = chatOperationName + " " + model
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

func (m Middleware) finish(
	ctx context.Context,
	span trace.Span,
	request *corechat.Request,
	response *corechat.Response,
	err error,
	elapsed time.Duration,
) {
	defer span.End()
	span.SetAttributes(responseAttributes(response)...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	m.recordMetrics(ctx, request, response, elapsed, err)
}

func (m Middleware) recordMetrics(
	ctx context.Context,
	request *corechat.Request,
	response *corechat.Response,
	elapsed time.Duration,
	err error,
) {
	attrs := metricAttributes(request, response)
	if err != nil {
		attrs = append(attrs, errorTypeAttribute(err))
	}
	m.duration.Record(ctx, elapsed.Seconds(),
		genaiconv.OperationNameChat,
		genaiconv.ProviderNameAttr(m.provider),
		attrs...,
	)
	if err != nil || response == nil {
		return
	}
	if response.Metadata == nil {
		return
	}
	if response.Metadata.Usage.InputTokens > 0 {
		m.tokens.Record(ctx, response.Metadata.Usage.InputTokens,
			genaiconv.OperationNameChat,
			genaiconv.ProviderNameAttr(m.provider),
			genaiconv.TokenTypeInput,
			attrs...,
		)
	}
	if response.Metadata.Usage.OutputTokens > 0 {
		m.tokens.Record(ctx, response.Metadata.Usage.OutputTokens,
			genaiconv.OperationNameChat,
			genaiconv.ProviderNameAttr(m.provider),
			genaiconv.TokenTypeOutput,
			attrs...,
		)
	}
}

func requestAttributes(request *corechat.Request) []attribute.KeyValue {
	if request == nil {
		return nil
	}
	options := request.Options
	var attrs []attribute.KeyValue
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

func responseAttributes(response *corechat.Response) []attribute.KeyValue {
	if response == nil {
		return nil
	}
	var attrs []attribute.KeyValue
	if response.Metadata != nil {
		if response.Metadata.ID != "" {
			attrs = append(attrs, semconv.GenAIResponseID(response.Metadata.ID))
		}
		if response.Metadata.Model != "" {
			attrs = append(attrs, semconv.GenAIResponseModel(response.Metadata.Model))
		}
		if response.Metadata.Usage.InputTokens > 0 {
			attrs = append(attrs, semconv.GenAIUsageInputTokensKey.Int64(response.Metadata.Usage.InputTokens))
		}
		if response.Metadata.Usage.OutputTokens > 0 {
			attrs = append(attrs, semconv.GenAIUsageOutputTokensKey.Int64(response.Metadata.Usage.OutputTokens))
		}
	}
	if response.Output != nil && response.Output.FinishReason != "" {
		attrs = append(attrs, semconv.GenAIResponseFinishReasons(response.Output.FinishReason.String()))
	}
	return attrs
}

func metricAttributes(request *corechat.Request, response *corechat.Response) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	if model := requestModel(request); model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(model))
	}
	responseModel := ""
	if response != nil && response.Metadata != nil {
		responseModel = response.Metadata.Model
	}
	if responseModel == "" {
		responseModel = requestModel(request)
	}
	if responseModel != "" {
		attrs = append(attrs, semconv.GenAIResponseModel(responseModel))
	}
	return attrs
}

func requestModel(request *corechat.Request) string {
	if request == nil {
		return ""
	}
	return request.Options.Model
}

func hasGeneratedContent(response *corechat.Response) bool {
	if response == nil {
		return false
	}
	return response.Output != nil && response.Output.Message != nil && len(response.Output.Message.Parts) > 0
}

func errorTypeAttribute(err error) attribute.KeyValue {
	switch {
	case errors.Is(err, context.Canceled):
		return semconv.ErrorTypeKey.String(errorTypeContextCanceled)
	case errors.Is(err, context.DeadlineExceeded):
		return semconv.ErrorTypeKey.String(errorTypeDeadlineExceeded)
	case errors.Is(err, corechat.ErrInvalidRequest):
		return semconv.ErrorTypeKey.String(errorTypeInvalidRequest)
	case errors.Is(err, corechat.ErrInvalidResponse):
		return semconv.ErrorTypeKey.String(errorTypeInvalidResponse)
	case errors.Is(err, corechat.ErrInvalidMessage):
		return semconv.ErrorTypeKey.String(errorTypeInvalidMessage)
	case errors.Is(err, corechat.ErrInvalidPart):
		return semconv.ErrorTypeKey.String(errorTypeInvalidPart)
	case errors.Is(err, corechat.ErrInvalidToolCall):
		return semconv.ErrorTypeKey.String(errorTypeInvalidToolCall)
	case errors.Is(err, corechat.ErrInvalidToolResult):
		return semconv.ErrorTypeKey.String(errorTypeInvalidToolResult)
	case errors.Is(err, corechat.ErrInvalidToolDefinition):
		return semconv.ErrorTypeKey.String(errorTypeInvalidToolDefinition)
	case errors.Is(err, corechat.ErrInvalidOutputFormat):
		return semconv.ErrorTypeKey.String(errorTypeInvalidOutputFormat)
	case errors.Is(err, corechat.ErrInvalidOptions):
		return semconv.ErrorTypeKey.String(errorTypeInvalidOptions)
	case errors.Is(err, corechat.ErrInvalidUsage):
		return semconv.ErrorTypeKey.String(errorTypeInvalidUsage)
	case errors.Is(err, ErrNilStream):
		return semconv.ErrorTypeKey.String(errorTypeNilStream)
	default:
		return semconv.ErrorType(err)
	}
}
