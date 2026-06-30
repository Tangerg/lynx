package runtime

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// publishEvent dispatches via the platform's multicast listener and
// the per-process multicast (populated from process-scope EventListener
// extensions). Either may be nil — the function tolerates that.
func (p *AgentProcess) publishEvent(e event.Event) {
	if p.platform != nil {
		p.platform.publish(e)
	}
	if p.processEvents != nil && e != nil {
		p.processEvents.OnEvent(e)
	}
}

// baseEvent stamps a fresh [event.BaseEvent] tagged with this process's
// id. Convenience used by every event the runtime emits — keeps the
// per-event struct literals one liner short.
func (p *AgentProcess) baseEvent() event.BaseEvent {
	return event.NewBaseEvent(p.id)
}

// publishAny accepts the type-erased event used by ProcessContext.Publish.
func (p *AgentProcess) publishAny(e any) {
	ev, ok := e.(event.Event)
	if !ok {
		return
	}
	p.publishEvent(ev)
}

// Tracing attribute keys shared between process- and action-level
// spans. Centralized at the AgentProcess scope (where they originate)
// because external listeners — dashboards, exporters — key off the
// stable string values. Treat as schema; renaming breaks consumers.
const (
	attrAgentName = "gen_ai.agent.name"
	attrProcessID = "agent.process.id"
)

// startTickSpan creates a span scoped to one tick.
func (p *AgentProcess) startTickSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return core.AgentTracer().Start(ctx, name,
		trace.WithAttributes(
			attribute.String(attrAgentName, p.agent.Name),
			attribute.String(attrProcessID, p.id),
		),
	)
}

// finishSpanWithError records err on span and sets the OTel error status.
func finishSpanWithError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
