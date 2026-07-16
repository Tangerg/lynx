package toolport

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

const (
	// ToolRoleCoding is the role the main chat agent declares: the full coding
	// tool set plus the task delegation tool.
	ToolRoleCoding = "coding"
	// ToolRoleSubtask is the role a delegated sub-agent declares: the same
	// coding tools without task, so delegation cannot recurse.
	ToolRoleSubtask = "subtask"
)

// ToolResolver resolves role-specific tool groups and accepts the task tool
// that can only be built after the agent engine exists.
type ToolResolver interface {
	core.ToolGroupResolver
	UseTaskTool(tools.Tool)
}

// MCPStatusReader reads live status for configured MCP servers.
type MCPStatusReader interface {
	Statuses() []mcpserver.ConnectionStatus
}

// MCPToolCatalog lists tools advertised by live MCP server connections.
type MCPToolCatalog interface {
	Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error)
}

// MCPConnectionCommands operates on an already configured MCP server's live
// connection.
type MCPConnectionCommands interface {
	Reconnect(ctx context.Context, name string) error
	Authorize(ctx context.Context, name string) error
}

// MCPRegistryCommands probes and applies live MCP server registry changes.
type MCPRegistryCommands interface {
	Probe(ctx context.Context, config mcpserver.LiveConfig) error
	Configure(ctx context.Context, config mcpserver.LiveConfig) error
	Remove(ctx context.Context, name string)
}
