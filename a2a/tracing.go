package a2a

import "go.opentelemetry.io/otel"

// a2aTracer is the package tracer. Spans are no-ops unless the application
// installs a TracerProvider (see doc/OBSERVABILITY.md); the a2a layer takes
// whatever is globally configured rather than accepting one by DI.
var a2aTracer = otel.Tracer("lynx/a2a")

// Attribute keys for A2A spans. Brand-neutral GenAI semconv where one
// exists, otherwise a bare domain key.
const (
	attrAgentName = "gen_ai.agent.name"
	attrTaskState = "a2a.task.state"
)
