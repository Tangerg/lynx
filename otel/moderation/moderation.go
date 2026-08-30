// Package moderation instruments Core moderation calls with OpenTelemetry.
package moderation

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

	coremoderation "github.com/Tangerg/scope/core/moderation"
)

const (
	instrumentationName          = "github.com/Tangerg/scope/otel/moderation"
	operationName                = "moderate"
	operationDurationMetric      = "gen_ai.client.operation.duration"
	operationDurationDescription = "GenAI client operation duration."
	operationDurationUnit        = "s"
	inputCountAttribute          = "gen_ai.request.input.count"
	errorCanceled                = "context.canceled"
	errorDeadline                = "context.deadline_exceeded"
	errorInvalidRequest          = "moderation.invalid_request"
	errorInvalidOutput           = "moderation.invalid_response"
)

var (
	ErrInvalidConfig = errors.New("otel/moderation: invalid config")
	ErrInvalidModel  = errors.New("otel/moderation: invalid model")
)

// MiddlewareConfig identifies the provider and optional OTel providers used by
// moderation instrumentation.
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

// Middleware observes moderation metadata without recording classified text or
// provider category names.
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

// Wrap decorates one moderation Model without changing its call semantics.
func (middleware Middleware) Wrap(next coremoderation.Model) (coremoderation.Model, error) {
	if lo.IsNil(middleware.tracer) || lo.IsNil(middleware.duration) {
		return nil, fmt.Errorf("%w: middleware must be constructed with NewMiddleware", ErrInvalidConfig)
	}
	if lo.IsNil(next) {
		return nil, fmt.Errorf("%w: value must not be nil", ErrInvalidModel)
	}
	return coremoderation.ModelFunc(func(ctx context.Context, request *coremoderation.Request) (*coremoderation.Response, error) {
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

func (middleware Middleware) spanName(request *coremoderation.Request) string {
	if request == nil || request.Options.Model == "" {
		return operationName
	}
	return operationName + " " + request.Options.Model
}

func (middleware Middleware) requestAttributes(request *coremoderation.Request) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		semconv.GenAIOperationNameKey.String(operationName),
		semconv.GenAIProviderNameKey.String(middleware.provider),
	}
	if request == nil {
		return attributes
	}
	if request.Options.Model != "" {
		attributes = append(attributes, semconv.GenAIRequestModel(request.Options.Model))
	}
	return append(attributes, attribute.Int(inputCountAttribute, len(request.Texts)))
}

func (middleware Middleware) finish(
	ctx context.Context,
	span trace.Span,
	request *coremoderation.Request,
	response *coremoderation.Response,
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
	request *coremoderation.Request,
	response *coremoderation.Response,
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
	case errors.Is(err, coremoderation.ErrInvalidRequest), errors.Is(err, coremoderation.ErrInvalidOptions):
		return semconv.ErrorTypeKey.String(errorInvalidRequest)
	case errors.Is(err, coremoderation.ErrInvalidResponse):
		return semconv.ErrorTypeKey.String(errorInvalidOutput)
	default:
		return semconv.ErrorType(err)
	}
}
