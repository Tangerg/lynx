package toolloop

import "go.opentelemetry.io/otel"

var toolTracer = otel.Tracer("lynx/tool")

const (
	attrToolName   = "gen_ai.tool.name"
	attrToolCallID = "gen_ai.tool.call.id"
)
