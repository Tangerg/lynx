package mcp

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// ToolInfo is one tool advertised by a connected server; the client-facing
// projection (workspace.mcp.listTools) of a remote tool descriptor. Name is the
// bare (un-prefixed) tool name; Server is the source. Tools reach the model
// under "<server>_<name>", but the wire view keeps the two fields separate.
type ToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema map[string]any
}

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

// schemaToMap renders an MCP tool's input schema as a generic object for the
// wire. A nil schema or a marshal failure yields nil rather than erroring a
// whole listing over one odd schema.
func schemaToMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return m
}
