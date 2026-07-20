package runtime

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Attribute keys are telemetry schema; rename only with exporter/dashboard migration.
const (
	agentTracerName   = "lynx/agent/runtime"
	spanRun           = "agent.run"
	spanAutoSnapshot  = "agent.auto_snapshot"
	attrAgentName     = "gen_ai.agent.name"
	attrProcessID     = "agent.process.id"
	attrProcessStatus = "agent.process.status"
)

var agentTracer = otel.Tracer(agentTracerName)

// publishEvent dispatches to both engine and process listener scopes.
func (p *Process) publishEvent(ctx context.Context, event event.Event) {
	if p.engine != nil {
		p.engine.publishContext(ctx, event)
	}
	if p.processEvents != nil && event != nil {
		p.processEvents.OnEvent(ctx, event)
	}
}

func (p *Process) eventHeader() event.Header {
	return event.NewHeader(p.id)
}

func (p *Process) publishCreated(ctx context.Context, bindings core.Bindings) {
	p.publishEvent(normalizeContext(ctx), event.ProcessCreated{
		Header:   p.eventHeader(),
		Bindings: bindings,
	})
}

// publishAny accepts the type-erased event surface exposed by ProcessContext.
func (p *Process) publishAny(ctx context.Context, value any) {
	eventValue, ok := value.(event.Event)
	if !ok {
		return
	}
	p.publishEvent(ctx, eventValue)
}

func (p *Process) startTickSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return agentTracer.Start(ctx, name,
		trace.WithAttributes(
			attribute.String(attrAgentName, p.agent().Name()),
			attribute.String(attrProcessID, p.id),
		),
	)
}

func finishSpanWithError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
