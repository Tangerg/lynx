// Package transcription instruments Core transcription calls with OpenTelemetry.
package transcription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	apiotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	coretranscription "github.com/Tangerg/scope/core/transcription"
)

const (
	instrumentationName          = "github.com/Tangerg/scope/otel/transcription"
	operationName                = "transcribe"
	operationDurationMetric      = "gen_ai.client.operation.duration"
	operationDurationDescription = "GenAI client operation duration."
	operationDurationUnit        = "s"
	errorCanceled                = "context.canceled"
	errorDeadline                = "context.deadline_exceeded"
	errorInvalidRequest          = "transcription.invalid_request"
	errorInvalidOutput           = "transcription.invalid_response"
)

var (
	ErrInvalidConfig = errors.New("otel/transcription: invalid config")
	ErrInvalidModel  = errors.New("otel/transcription: invalid model")
)

// MiddlewareConfig identifies the provider and optional OTel providers used by
// transcription instrumentation.
type MiddlewareConfig struct {
	Provider       string
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

// Validate checks construction inputs without resolving global providers.
func (config MiddlewareConfig) Validate() error {
	if strings.TrimSpace(config.Provider) == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidConfig)
	}
	return nil
}

// Middleware observes transcription calls without recording audio, transcript
// text, or language hints.
type Middleware struct {
	provider string
	tracer   trace.Tracer
	duration metric.Float64Histogram
}

// NewMiddleware snapshots provider identity and resolves nil OTel providers to
// the official process globals.
func NewMiddleware(config MiddlewareConfig) (Middleware, error) {
	if err := config.Validate(); err != nil {
		return Middleware{}, err
	}
	tracerProvider := config.TracerProvider
	if lo.IsNil(tracerProvider) {
		tracerProvider = apiotel.GetTracerProvider()
	}
	meterProvider := config.MeterProvider
	if lo.IsNil(meterProvider) {
		meterProvider = apiotel.GetMeterProvider()
	}
	duration, err := meterProvider.Meter(instrumentationName).Float64Histogram(
		operationDurationMetric,
		metric.WithDescription(operationDurationDescription),
		metric.WithUnit(operationDurationUnit),
	)
	if err != nil {
		return Middleware{}, fmt.Errorf("%w: create duration histogram: %w", ErrInvalidConfig, err)
	}
	return Middleware{
		provider: strings.ToLower(strings.TrimSpace(config.Provider)),
		tracer:   tracerProvider.Tracer(instrumentationName),
		duration: duration,
	}, nil
}

// Wrap decorates one transcription Model without changing its call semantics.
func (middleware Middleware) Wrap(next coretranscription.Model) (coretranscription.Model, error) {
	if lo.IsNil(middleware.tracer) || lo.IsNil(middleware.duration) {
		return nil, fmt.Errorf("%w: middleware must be constructed with NewMiddleware", ErrInvalidConfig)
	}
	if lo.IsNil(next) {
		return nil, fmt.Errorf("%w: value must not be nil", ErrInvalidModel)
	}
	return coretranscription.ModelFunc(func(ctx context.Context, request *coretranscription.Request) (*coretranscription.Response, error) {
		startedAt := time.Now()
		attributes := middleware.requestAttributes(request)
		spanCtx, span := middleware.tracer.Start(ctx, middleware.spanName(request),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attributes...),
		)
		response, err := next.Call(spanCtx, request)
		middleware.finish(spanCtx, span, request, response, err, time.Since(startedAt))
		return response, err
	}), nil
}

func (middleware Middleware) spanName(request *coretranscription.Request) string {
	if request == nil || request.Options.Model == "" {
		return operationName
	}
	return operationName + " " + request.Options.Model
}

func (middleware Middleware) requestAttributes(request *coretranscription.Request) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		semconv.GenAIOperationNameKey.String(operationName),
		semconv.GenAIProviderNameKey.String(middleware.provider),
	}
	if request != nil && request.Options.Model != "" {
		attributes = append(attributes, semconv.GenAIRequestModel(request.Options.Model))
	}
	return attributes
}

func (middleware Middleware) finish(
	ctx context.Context,
	span trace.Span,
	request *coretranscription.Request,
	response *coretranscription.Response,
	err error,
	elapsed time.Duration,
) {
	defer span.End()
	attributes := middleware.metricAttributes(request, response)
	if response != nil && response.Metadata != nil && response.Metadata.Model != "" {
		span.SetAttributes(semconv.GenAIResponseModel(response.Metadata.Model))
	}
	if err != nil {
		errorType := errorTypeAttribute(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(errorType)
		attributes = append(attributes, errorType)
	}
	middleware.duration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attributes...))
}

func (middleware Middleware) metricAttributes(
	request *coretranscription.Request,
	response *coretranscription.Response,
) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		semconv.GenAIOperationNameKey.String(operationName),
		semconv.GenAIProviderNameKey.String(middleware.provider),
	}
	if request != nil && request.Options.Model != "" {
		attributes = append(attributes, semconv.GenAIRequestModel(request.Options.Model))
	}
	model := ""
	if response != nil && response.Metadata != nil {
		model = response.Metadata.Model
	}
	if model == "" && request != nil {
		model = request.Options.Model
	}
	if model != "" {
		attributes = append(attributes, semconv.GenAIResponseModel(model))
	}
	return attributes
}

func errorTypeAttribute(err error) attribute.KeyValue {
	switch {
	case errors.Is(err, context.Canceled):
		return semconv.ErrorTypeKey.String(errorCanceled)
	case errors.Is(err, context.DeadlineExceeded):
		return semconv.ErrorTypeKey.String(errorDeadline)
	case errors.Is(err, coretranscription.ErrInvalidRequest), errors.Is(err, coretranscription.ErrInvalidOptions):
		return semconv.ErrorTypeKey.String(errorInvalidRequest)
	case errors.Is(err, coretranscription.ErrInvalidResponse):
		return semconv.ErrorTypeKey.String(errorInvalidOutput)
	default:
		return semconv.ErrorType(err)
	}
}
