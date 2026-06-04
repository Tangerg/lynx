package mcp

import (
	"go.opentelemetry.io/otel"
)

// mcpTracer is the package-level tracer for MCP client and server
// span emission. No-op overhead when no TracerProvider is installed —
// see doc/OBSERVABILITY.md §5.
var mcpTracer = otel.Tracer("lynx/mcp")

// MCP tool attribute key (GenAI semconv). Tool failures surface through
// the span status (Error) + RecordError, not a separate bool attribute.
const attrLynxMCPTool = "gen_ai.tool.name"
