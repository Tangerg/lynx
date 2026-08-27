package otel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/samber/lo"

	agent "github.com/Tangerg/scope/agent"
)

const instrumentationName = "github.com/Tangerg/scope/agent/otel"

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
// Observer values must be constructed with New and must not be copied after
// first use.
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
	if config.TracerProvider != nil && lo.IsNil(config.TracerProvider) {
		return nil, fmt.Errorf("%w: TracerProvider is typed nil", ErrInvalidConfig)
	}
	if config.MeterProvider != nil && lo.IsNil(config.MeterProvider) {
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
func (o *Observer) OnEvent(ctx context.Context, event agent.Event) {
	if o == nil || !event.Valid() {
		return
	}
	o.mu.Lock()
	closed := o.closed
	o.mu.Unlock()
	if closed {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch event.Name() {
	case agent.EventProcessStarted, agent.EventProcessRestored:
		o.startProcess(ctx, event)
	case agent.EventProcessFinished:
		o.finishProcess(ctx, event)
	case agent.EventStepStarted:
		o.startStep(event)
	case agent.EventStepFinished:
		o.finishStep(ctx, event)
	case agent.EventEffectStarted:
		o.startEffect(event)
	case agent.EventEffectFinished:
		o.finishEffect(ctx, event)
	case agent.EventDeltaDropped:
		o.recordDeltaDrop(ctx, event)
	default:
		o.addProcessEvent(event)
	}
}

// Close ends any incomplete spans and prevents further observation. It is
// idempotent; normally Engine.Close leaves no incomplete Process spans.
func (o *Observer) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	records := make([]spanRecord, 0, len(o.effects)+len(o.steps)+len(o.processes))
	for _, record := range o.effects {
		records = append(records, record)
	}
	for _, record := range o.steps {
		records = append(records, record)
	}
	for _, record := range o.processes {
		records = append(records, record)
	}
	clear(o.effects)
	clear(o.steps)
	clear(o.processes)
	o.mu.Unlock()
	endedAt := time.Now()
	for _, record := range records {
		record.span.SetStatus(codes.Error, "OpenTelemetry observer closed before span completion")
		record.span.End(trace.WithTimestamp(endedAt))
	}
}

func (o *Observer) startProcess(ctx context.Context, event agent.Event) {
	relation := event.Relation()
	parentContext := ctx
	if parentID, child := relation.ParentID(); child {
		o.mu.Lock()
		parent, found := o.processes[parentID]
		o.mu.Unlock()
		if found {
			parentContext = parent.ctx
		}
	}
	spanContext, span := o.tracer.Start(
		parentContext, "agent.process",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(processAttributes(event)...),
	)
	record := spanRecord{
		processID: event.ProcessID(), ctx: spanContext, span: span,
		startedAt: event.OccurredAt(),
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		span.End(trace.WithTimestamp(event.OccurredAt()))
		return
	}
	if _, exists := o.processes[event.ProcessID()]; exists {
		o.mu.Unlock()
		span.End(trace.WithTimestamp(event.OccurredAt()))
		return
	}
	o.processes[event.ProcessID()] = record
	o.mu.Unlock()
	attributes := metric.WithAttributes(deploymentMetricAttributes(event)...)
	o.processStarts.Add(ctx, 1, attributes)
}

func (o *Observer) finishProcess(ctx context.Context, event agent.Event) {
	payload := decodePayload(event)
	o.mu.Lock()
	record, found := o.processes[event.ProcessID()]
	if found {
		delete(o.processes, event.ProcessID())
	}
	o.mu.Unlock()
	attributes := append(deploymentMetricAttributes(event),
		attribute.String("agent.process.status", payload.ProcessStatus.String()),
		attribute.String("agent.process.cause", payload.TerminationCause.String()),
	)
	o.processExits.Add(ctx, 1, metric.WithAttributes(attributes...))
	if !found {
		return
	}
	record.span.SetAttributes(
		attribute.String("agent.process.status", payload.ProcessStatus.String()),
		attribute.String("agent.process.cause", payload.TerminationCause.String()),
	)
	if processStatusIsError(payload.ProcessStatus) {
		record.span.SetStatus(codes.Error, payload.TerminationCause.String())
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
}

func (o *Observer) startStep(event agent.Event) {
	sequence, ok := event.StepSequence()
	if !ok {
		return
	}
	o.mu.Lock()
	process, found := o.processes[event.ProcessID()]
	if !found || o.closed {
		o.mu.Unlock()
		return
	}
	key := stepKey{processID: event.ProcessID(), sequence: sequence}
	if _, exists := o.steps[key]; exists {
		o.mu.Unlock()
		return
	}
	ctx, span := o.tracer.Start(
		process.ctx, "agent.step",
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(uint64Attribute("agent.step.sequence", sequence)),
	)
	o.steps[key] = spanRecord{
		processID: event.ProcessID(), ctx: ctx, span: span, startedAt: event.OccurredAt(),
	}
	o.mu.Unlock()
}

func (o *Observer) finishStep(ctx context.Context, event agent.Event) {
	sequence, ok := event.StepSequence()
	if !ok {
		return
	}
	key := stepKey{processID: event.ProcessID(), sequence: sequence}
	o.mu.Lock()
	record, found := o.steps[key]
	if found {
		delete(o.steps, key)
	}
	o.mu.Unlock()
	if !found {
		return
	}
	payload := decodePayload(event)
	record.span.SetAttributes(attribute.String("agent.step.status", payload.StepStatus.String()))
	if payload.StepStatus == agent.StepStatusFailed {
		record.span.SetStatus(codes.Error, "Execution Step failed")
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
	o.stepDuration.Record(
		ctx, elapsedMilliseconds(record.startedAt, event.OccurredAt()),
		metric.WithAttributes(attribute.String("agent.step.status", payload.StepStatus.String())),
	)
}

func (o *Observer) startEffect(event agent.Event) {
	effectID, ok := event.EffectID()
	if !ok {
		return
	}
	payload := decodePayload(event)
	o.mu.Lock()
	process, found := o.processes[event.ProcessID()]
	if !found || o.closed {
		o.mu.Unlock()
		return
	}
	if _, exists := o.effects[effectID]; exists {
		o.mu.Unlock()
		return
	}
	ctx, span := o.tracer.Start(
		process.ctx, "agent.effect",
		trace.WithTimestamp(event.OccurredAt()),
		trace.WithAttributes(
			attribute.String("agent.effect.id", effectID.String()),
			attribute.String("agent.effect.target", payload.EffectTarget.String()),
		),
	)
	o.effects[effectID] = spanRecord{
		processID: event.ProcessID(), ctx: ctx, span: span, startedAt: event.OccurredAt(),
	}
	o.mu.Unlock()
}

func (o *Observer) finishEffect(ctx context.Context, event agent.Event) {
	effectID, ok := event.EffectID()
	if !ok {
		return
	}
	o.mu.Lock()
	record, found := o.effects[effectID]
	if found {
		delete(o.effects, effectID)
	}
	o.mu.Unlock()
	if !found {
		return
	}
	payload := decodePayload(event)
	record.span.SetAttributes(
		attribute.String("agent.effect.target", payload.EffectTarget.String()),
		attribute.String("agent.effect.status", payload.SettlementStatus.String()),
	)
	if payload.SettlementStatus != agent.SettlementStatusSucceeded {
		record.span.SetStatus(codes.Error, "Effect attempt "+payload.SettlementStatus.String())
	}
	record.span.End(trace.WithTimestamp(event.OccurredAt()))
	o.effectDuration.Record(
		ctx, elapsedMilliseconds(record.startedAt, event.OccurredAt()),
		metric.WithAttributes(
			attribute.String("agent.effect.target", payload.EffectTarget.String()),
			attribute.String("agent.effect.status", payload.SettlementStatus.String()),
		),
	)
}

func (o *Observer) recordDeltaDrop(ctx context.Context, event agent.Event) {
	payload := decodePayload(event)
	if payload.DroppedDeltaCount > 0 {
		o.deltaDrops.Add(ctx, saturatingInt64(payload.DroppedDeltaCount))
	}
	o.addProcessEvent(event)
}

func (o *Observer) addProcessEvent(event agent.Event) {
	o.mu.Lock()
	record, found := o.processes[event.ProcessID()]
	o.mu.Unlock()
	if !found {
		return
	}
	attributes := []attribute.KeyValue{
		uint64Attribute("agent.process.event_sequence", event.ProcessSequence()),
		attribute.String("agent.event.phase", event.Phase().String()),
	}
	if step, ok := event.StepSequence(); ok {
		attributes = append(attributes, uint64Attribute("agent.step.sequence", step))
	}
	if effectID, ok := event.EffectID(); ok {
		attributes = append(attributes, attribute.String("agent.effect.id", effectID.String()))
	}
	record.span.AddEvent(
		event.Name(), trace.WithTimestamp(event.OccurredAt()), trace.WithAttributes(attributes...),
	)
}

type eventPayload struct {
	ProcessStatus     agent.Status           `json:"process_status"`
	TerminationCause  agent.TerminationCause `json:"termination_cause"`
	StepStatus        agent.StepStatus       `json:"step_status"`
	EffectTarget      agent.EffectTarget     `json:"effect_target"`
	SettlementStatus  agent.SettlementStatus `json:"settlement_status"`
	DroppedDeltaCount uint64                 `json:"dropped_delta_count"`
}

func uint64Attribute(name string, value uint64) attribute.KeyValue {
	return attribute.String(name, strconv.FormatUint(value, 10))
}

func saturatingInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
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

func processStatusIsError(status agent.Status) bool {
	switch status {
	case agent.StatusFailed, agent.StatusTimedOut, agent.StatusKilled:
		return true
	default:
		return false
	}
}

var _ agent.EventListener = (*Observer)(nil)
