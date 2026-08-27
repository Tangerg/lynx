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

	corechat "github.com/Tangerg/lynx/core/chat"
)

const instrumentationName = "github.com/Tangerg/lynx/otel/chat"

var (
	// ErrInvalidConfig reports a missing or malformed provider identity.
	ErrInvalidConfig = errors.New("otel/chat: invalid config")
	// ErrNilStream reports a wrapped Streamer that returned a nil sequence.
	ErrNilStream = errors.New("otel/chat: nil stream sequence")
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

// Validate verifies the provider identity required by chat instrumentation.
func (c MiddlewareConfig) Validate() error {
	if strings.TrimSpace(c.Provider) == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidConfig)
	}
	return nil
}

// Middleware adds GenAI spans and metrics to synchronous and streaming
// chat capabilities. It is immutable after construction and safe for
// concurrent use.
type Middleware struct {
	provider string
	tracer   trace.Tracer
	duration genaiconv.ClientOperationDuration
	tokens   genaiconv.ClientTokenUsage
}

// NewMiddleware constructs chat instrumentation. Provider is required at the
// composition root instead of being added to the Core Model contract.
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

	return Middleware{
		provider: provider,
		tracer:   tracerProvider.Tracer(instrumentationName),
		duration: duration,
		tokens:   tokens,
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
					span.AddEvent("first_token_received")
					firstToken = true
				}
				if chunk != nil {
					if accumulationErr := accumulator.Add(chunk); accumulationErr != nil {
						span.AddEvent("gen_ai.stream.accumulation_error",
							trace.WithAttributes(semconv.ErrorTypeKey.String(errorType(accumulationErr))),
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

func (m Middleware) start(
	ctx context.Context,
	request *corechat.Request,
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

func responseAttributes(response *corechat.Response) []attribute.KeyValue {
	if response == nil {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, 5)
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
	attrs := make([]attribute.KeyValue, 0, 2)
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

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
