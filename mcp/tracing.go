package mcp

import (
	"go.opentelemetry.io/otel"
)

// mcpTracer is the package-level tracer for MCP client and server span
// emission. It is a no-op when no TracerProvider is installed.
var mcpTracer = otel.Tracer("lynx/mcp")

// MCP tool attribute key (GenAI semconv). Tool failures surface through
// the span status (Error) + RecordError, not a separate bool attribute.
const attrToolName = "gen_ai.tool.name"
