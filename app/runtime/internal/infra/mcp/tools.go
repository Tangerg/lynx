package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own.
func sourceTools(ctx context.Context, src lynxmcp.ToolSource) ([]tools.Tool, error) {
	return lynxmcp.Tools(ctx, []lynxmcp.ToolSource{src}, lynxmcp.ToolOptions{
		Naming: func(server string, tool *sdkmcp.Tool) string {
			return mcpserver.ToolName(server, tool.Name)
		},
		Concurrency: lynxmcp.AnnotatedReadOnlyConcurrency,
	})
}

// inputSchema converts the SDK's open schema representation at the MCP
// boundary. Missing or malformed advertised schemas fail the catalog read
// instead of being silently presented as schema-less tools.
func inputSchema(schema any) (mcpserver.InputSchema, error) {
	parsed, err := mcpserver.NewInputSchema(schema)
	if err != nil {
		return mcpserver.InputSchema{}, err
	}
	return parsed, nil
}
