package mcp

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

// sourceTools lists one MCP source's model-facing tools. Isolated per source so
// a single server's tools/list failure stays its own.
func sourceTools(ctx context.Context, src lynxmcp.ToolSource) ([]toolcontract.Tool, error) {
	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{src}, lynxmcp.ToolsConfig{
		Naming: func(server, toolName string) string {
			return mcpserver.ToolName(server, toolName)
		},
		Concurrency: lynxmcp.AnnotatedReadOnlyConcurrency,
	})
	if err != nil {
		return nil, err
	}
	if err := validateSourceToolMaterial(src.Name, tools); err != nil {
		return nil, err
	}
	return tools, nil
}

func validateSourceToolMaterial(server string, tools []toolcontract.Tool) error {
	if err := mcpserver.ValidateRemoteToolCount(len(tools)); err != nil {
		return fmt.Errorf("mcp: validate tools from server %q: %w", server, err)
	}
	for _, tool := range tools {
		definition := tool.Definition()
		if err := mcpserver.ValidateRemoteToolMaterial(definition.Description, definition.InputSchema); err != nil {
			return fmt.Errorf("mcp: validate tool %q from server %q: %w", definition.Name, server, err)
		}
	}
	return nil
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
