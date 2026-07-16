package runtime

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var agentTracer = otel.Tracer("lynx/agent/runtime")

// Attribute keys are telemetry schema; rename only with exporter/dashboard migration.
const (
	attrAgentName = "gen_ai.agent.name"
	attrProcessID = "agent.process.id"
)

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
