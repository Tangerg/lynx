package mcp

import (
	"go.opentelemetry.io/otel"
)

// mcpTracer is the package-level tracer for MCP client and server
// span emission. No-op overhead when no TracerProvider is installed —
// see doc/OBSERVABILITY.md §5.
var mcpTracer = otel.Tracer("lynx/mcp")

// Lynx MCP attribute keys per doc/OBSERVABILITY.md §3.3.
const (
	attrLynxMCPTool    = "lynx.tool.name"
	attrLynxMCPIsError = "lynx.mcp.tool.is_error"
)
