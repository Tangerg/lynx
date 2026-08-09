package otel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	agent "github.com/Tangerg/lynx/agent"
)

const instrumentationName = "github.com/Tangerg/lynx/agent/otel"

// ErrInvalidConfig reports a typed-nil provider or unusable instrument setup.
var ErrInvalidConfig = errors.New("agent otel: invalid configuration")

// Config selects the official OpenTelemetry providers used by Observer. A nil
// provider uses the corresponding OpenTelemetry global provider.
type Config struct {
	// TracerProvider creates Process, Step, and Effect spans. Nil uses the
	// OpenTelemetry global provider.
	TracerProvider trace.TracerProvider

	// MeterProvider creates Process counters and Step/Effect duration
	// histograms. Nil uses the OpenTelemetry global provider.
	MeterProvider metric.MeterProvider
}

// Observer projects immutable Framework Event facts into OpenTelemetry spans
// and metrics. It implements agent.EventListener and is safe for concurrent
// calls. Observer never receives Process behavior or application state.
type Observer struct {
	tracer trace.Tracer

	processStarts  metric.Int64Counter
	processExits   metric.Int64Counter
	stepDuration   metric.Float64Histogram
	effectDuration metric.Float64Histogram
	deltaDrops     metric.Int64Counter

	mu        sync.Mutex
	processes map[agent.ProcessID]spanRecord
	steps     map[stepKey]spanRecord
	effects   map[agent.EffectID]spanRecord
	closed    bool
}

type spanRecord struct {
	processID agent.ProcessID
	ctx       context.Context
	span      trace.Span
	startedAt time.Time
}

type stepKey struct {
	processID agent.ProcessID
	sequence  uint64
}

// New validates providers and constructs one isolated Observer.
func New(config Config) (*Observer, error) {
	if typedNil(config.TracerProvider) {
		return nil, fmt.Errorf("%w: TracerProvider is typed nil", ErrInvalidConfig)
	}
	if typedNil(config.MeterProvider) {
		return nil, fmt.Errorf("%w: MeterProvider is typed nil", ErrInvalidConfig)
	}
	tracerProvider := config.TracerProvider
	if tracerProvider == nil {
		tracerProvider = otel.GetTracerProvider()
	}
	meterProvider := config.MeterProvider
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}
	meter := meterProvider.Meter(instrumentationName)
	processStarts, err := meter.Int64Counter(
		"agent.process.starts",
		metric.WithDescription("Agent Process executions started or restored."),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: process starts counter: %w", ErrInvalidConfig, err)
	}
	processExits, err := meter.Int64Counter(
		"agent.process.exits",
		metric.WithDescription("Agent Process terminal outcomes."),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: process exits counter: %w", ErrInvalidConfig, err)
	}
	stepDuration, err := meter.Float64Histogram(
		"agent.step.duration",
		metric.WithDescription("Execution Step wall-clock duration."),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: step duration histogram: %w", ErrInvalidConfig, err)
	}
	effectDuration, err := meter.Float64Histogram(
		"agent.effect.duration",
		metric.WithDescription("Framework or Dispatcher Effect attempt duration."),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: effect duration histogram: %w", ErrInvalidConfig, err)
	}
	deltaDrops, err := meter.Int64Counter(
		"agent.delta.dropped",
		metric.WithDescription("Best-effort Delta increments dropped before observation."),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: delta drop counter: %w", ErrInvalidConfig, err)
	}
	return &Observer{
		tracer:        tracerProvider.Tracer(instrumentationName),
		processStarts: processStarts, processExits: processExits,
		stepDuration: stepDuration, effectDuration: effectDuration, deltaDrops: deltaDrops,
		processes: make(map[agent.ProcessID]spanRecord),
		steps:     make(map[stepKey]spanRecord), effects: make(map[agent.EffectID]spanRecord),
	}, nil
}

// OnEvent records one valid Framework fact. Invalid events and calls after
// Close are ignored because observation cannot affect Process correctness.
func (observer *Observer) OnEvent(ctx context.Context, event agent.Event) {
	if observer == nil || !event.Valid() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch event.Name() {
	case agent.EventProcessStarted, agent.EventProcessRestored:
		observer.startProcess(ctx, event)
	case agent.EventProcessFinished:
		observer.finishProcess(ctx, event)
	case agent.EventStepStarted:
		observer.startStep(event)
	case agent.EventStepFinished:
		observer.finishStep(ctx, event)
	case agent.EventEffectStarted:
		observer.startEffect(event)
	case agent.EventEffectFinished:
		observer.finishEffect(ctx, event)
	case agent.EventDeltaDropped:
		observer.recordDeltaDrop(ctx, event)
	default:
		observer.addProcessEvent(event)
	}
}

// Close ends any incomplete spans and prevents further observation. It is
// idempotent; normally Engine.Close leaves no incomplete Process spans.
func (observer *Observer) Close() {
	if observer == nil {
		return
	}
	observer.mu.Lock()
	if observer.closed {
		observer.mu.Unlock()
		return
	}
	observer.closed = true
	records := make([]spanRecord, 0, len(observer.effects)+len(observer.steps)+len(observer.processes))
	for _, record := range observer.effects {
		records = append(records, record)
	}
	for _, record := range observer.steps {
		records = append(records, record)
	}
	for _, record := range observer.processes {
		records = append(records, record)
	}
	clear(observer.effects)
	clear(observer.steps)
	clear(observer.processes)
	observer.mu.Unlock()
	endedAt := time.Now()
	for _, record := range records {
		record.span.SetStatus(codes.Error, "OpenTelemetry observer closed before span completion")
		record.span.End(trace.WithTimestamp(endedAt))
	}
}

func (observer *Observer) startProcess(ctx context.Context, event agent.Event) {
	relation := event.Relation()
	parentContext := ctx
	if parentID, child := relation.ParentID(); child {
		observer.mu.Lock()
		parent, found := observer.processes[parentID]
		observer.mu.Unlock()
		if found {
			parentContext = parent.ctx
		}
	}
	spanContext, span := observer.tracer.Start(
		parentContext, "agent.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(processAttributes(event)...),
	)
	record := spanRecord{
		processID: event.ProcessID(), ctx: spanContext, span: span,
		startedAt: event.OccurredAt(),
	}
	observer.mu.Lock()
	if observer.closed {
		observer.mu.Unlock()
		span.End(trace.WithTimestamp(event.OccurredAt()))
		return
	}
	if _, exists := observer.processes[event.ProcessID()]; exists {
		observer.mu.Unlock()
		span.End(trace.WithTimestamp(event.OccurredAt()))
		return
	}
	observer.processes[event.ProcessID()] = record
	observer.mu.Unlock()
	attributes := metric.WithAttributes(deploymentMetricAttributes(event)...)
	observer.processStarts.Add(ctx, 1, attributes)
}

func (observer *Observer) finishProcess(ctx context.Context, event agent.Event) {
	payload := decodePayload(event)
	observer.mu.Lock()
	record, found := observer.processes[event.ProcessID()]
	if found {
		delete(observer.processes, event.ProcessID())
	}
	observer.mu.Unlock()
	attributes := append(deploymentMetricAttributes(event),
		attribute.String("agent.process.status", payload.ProcessStatus),
		attribute.String("agent.process.cause", payload.TerminationCause),
	)
	observer.processExits.Add(ctx, 1, metric.WithAttributes(attributes...))
	if !found {
		return
	}
	record.span.SetAttributes(
		attribute.String("agent.process.status", payload.ProcessStatus),
		attribute.String("agent.process.cause", payload.TerminationCause),
	)
	if processStatusIsError(payload.ProcessStatus) {
		record.span.SetStatus(codes.Error, payload.TerminationCause)
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
}

func (observer *Observer) startStep(event agent.Event) {
	sequence, ok := event.StepSequence()
	if !ok {
		return
	}
	observer.mu.Lock()
	process, found := observer.processes[event.ProcessID()]
	if !found || observer.closed {
		observer.mu.Unlock()
		return
	}
	key := stepKey{processID: event.ProcessID(), sequence: sequence}
	if _, exists := observer.steps[key]; exists {
		observer.mu.Unlock()
		return
	}
	ctx, span := observer.tracer.Start(
		process.ctx, "agent.step",
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(attribute.Int64("agent.step.sequence", int64(sequence))),
	)
	observer.steps[key] = spanRecord{
		processID: event.ProcessID(), ctx: ctx, span: span, startedAt: event.OccurredAt(),
	}
	observer.mu.Unlock()
}

func (observer *Observer) finishStep(ctx context.Context, event agent.Event) {
	sequence, ok := event.StepSequence()
	if !ok {
		return
	}
	key := stepKey{processID: event.ProcessID(), sequence: sequence}
	observer.mu.Lock()
	record, found := observer.steps[key]
	if found {
		delete(observer.steps, key)
	}
	observer.mu.Unlock()
	if !found {
		return
	}
	payload := decodePayload(event)
	record.span.SetAttributes(attribute.String("agent.step.status", payload.StepStatus))
	if payload.StepStatus == "failed" {
		record.span.SetStatus(codes.Error, "Execution Step failed")
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
	observer.stepDuration.Record(
		ctx, elapsedMilliseconds(record.startedAt, event.OccurredAt()),
		metric.WithAttributes(attribute.String("agent.step.status", payload.StepStatus)),
	)
}

func (observer *Observer) startEffect(event agent.Event) {
	effectID, ok := event.EffectID()
	if !ok {
		return
	}
	payload := decodePayload(event)
	observer.mu.Lock()
	process, found := observer.processes[event.ProcessID()]
	if !found || observer.closed {
		observer.mu.Unlock()
		return
	}
	if _, exists := observer.effects[effectID]; exists {
		observer.mu.Unlock()
		return
	}
	ctx, span := observer.tracer.Start(
		process.ctx, "agent.effect",
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(
			attribute.String("agent.effect.id", effectID.String()),
			attribute.String("agent.effect.target", payload.EffectTarget),
		),
	)
	observer.effects[effectID] = spanRecord{
		processID: event.ProcessID(), ctx: ctx, span: span, startedAt: event.OccurredAt(),
	}
	observer.mu.Unlock()
}

func (observer *Observer) finishEffect(ctx context.Context, event agent.Event) {
	effectID, ok := event.EffectID()
	if !ok {
		return
	}
	observer.mu.Lock()
	record, found := observer.effects[effectID]
	if found {
		delete(observer.effects, effectID)
	}
	observer.mu.Unlock()
	if !found {
		return
	}
	payload := decodePayload(event)
	record.span.SetAttributes(
		attribute.String("agent.effect.target", payload.EffectTarget),
		attribute.String("agent.effect.status", payload.SettlementStatus),
	)
	if payload.SettlementStatus != "succeeded" {
		record.span.SetStatus(codes.Error, "Effect attempt "+payload.SettlementStatus)
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
	observer.effectDuration.Record(
		ctx, elapsedMilliseconds(record.startedAt, event.OccurredAt()),
		metric.WithAttributes(
			attribute.String("agent.effect.target", payload.EffectTarget),
			attribute.String("agent.effect.status", payload.SettlementStatus),
		),
	)
}

func (observer *Observer) recordDeltaDrop(ctx context.Context, event agent.Event) {
	payload := decodePayload(event)
	if payload.DroppedDeltaCount > 0 {
		observer.deltaDrops.Add(ctx, payload.DroppedDeltaCount)
	}
	observer.addProcessEvent(event)
}

func (observer *Observer) addProcessEvent(event agent.Event) {
	observer.mu.Lock()
	record, found := observer.processes[event.ProcessID()]
	observer.mu.Unlock()
	if !found {
		return
	}
	attributes := []attribute.KeyValue{
		attribute.Int64("agent.process.event_sequence", int64(event.ProcessSequence())),
		attribute.String("agent.event.phase", event.Phase().String()),
	}
	if step, ok := event.StepSequence(); ok {
		attributes = append(attributes, attribute.Int64("agent.step.sequence", int64(step)))
	}
	if effectID, ok := event.EffectID(); ok {
		attributes = append(attributes, attribute.String("agent.effect.id", effectID.String()))
	}
	record.span.AddEvent(
		event.Name(), trace.WithTimestamp(event.OccurredAt()), trace.WithAttributes(attributes...),
	)
}

type eventPayload struct {
	ProcessStatus     string `json:"process_status"`
	TerminationCause  string `json:"termination_cause"`
	StepStatus        string `json:"step_status"`
	EffectTarget      string `json:"effect_target"`
	SettlementStatus  string `json:"settlement_status"`
	DroppedDeltaCount int64  `json:"dropped_delta_count"`
}

func decodePayload(event agent.Event) eventPayload {
	var payload eventPayload
	_ = json.Unmarshal(event.Payload(), &payload)
	return payload
}

func processAttributes(event agent.Event) []attribute.KeyValue {
	reference := event.DeploymentRef()
	relation := event.Relation()
	values := []attribute.KeyValue{
		attribute.String("agent.process.id", event.ProcessID().String()),
		attribute.String("agent.process.root_id", relation.RootID().String()),
		attribute.Int64("agent.process.depth", int64(relation.Depth())),
		attribute.String("agent.deployment.name", reference.Name()),
		attribute.String("agent.deployment.version", reference.Version()),
		attribute.String("agent.deployment.digest", reference.Digest().String()),
	}
	if parentID, child := relation.ParentID(); child {
		values = append(values, attribute.String("agent.process.parent_id", parentID.String()))
	}
	return values
}

func deploymentMetricAttributes(event agent.Event) []attribute.KeyValue {
	reference := event.DeploymentRef()
	return []attribute.KeyValue{
		attribute.String("agent.deployment.name", reference.Name()),
		attribute.String("agent.deployment.version", reference.Version()),
	}
}

func elapsedMilliseconds(startedAt, finishedAt time.Time) float64 {
	if finishedAt.Before(startedAt) {
		return 0
	}
	return float64(finishedAt.Sub(startedAt)) / float64(time.Millisecond)
}

func processStatusIsError(status string) bool {
	switch status {
	case "failed", "timed_out", "killed":
		return true
	default:
		return false
	}
}

func typedNil(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ agent.EventListener = (*Observer)(nil)
