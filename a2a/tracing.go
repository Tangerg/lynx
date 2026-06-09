package a2a

import "go.opentelemetry.io/otel"

// a2aTracer is the package tracer. Spans are no-ops unless the application
// installs a TracerProvider (see doc/OBSERVABILITY.md); the a2a layer takes
// whatever is globally configured rather than accepting one by DI.
var a2aTracer = otel.Tracer("lynx/a2a")

// attrAgentName tags an A2A client span with the remote agent's name —
// brand-neutral GenAI semconv.
const attrAgentName = "gen_ai.agent.name"
