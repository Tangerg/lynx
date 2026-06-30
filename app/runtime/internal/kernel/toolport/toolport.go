package toolport

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

const (
	// ToolRoleCoding is the role the main chat agent declares: the full coding
	// tool set plus the task delegation tool.
	ToolRoleCoding = "coding"
	// ToolRoleSubtask is the role a delegated sub-agent declares: the same
	// coding tools without task, so delegation cannot recurse.
	ToolRoleSubtask = "subtask"
)

// ToolResolver is the tool-group resolver the engine needs from the outer tool
// capability adapter. The task tool is built by the engine because it spawns
// sub-agents on the engine's platform, then injected back into the resolver.
type ToolResolver interface {
	core.ToolGroupResolver
	SetTask(chat.Tool)
}

// MCPToolInfo is one tool advertised by a connected MCP server.
type MCPToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema map[string]any
}

// MCPServerStatus is the per-server connection state exposed by the live MCP
// control plane.
type MCPServerStatus struct {
	Name   string
	Status string
	Err    error
}

type (
	McpToolInfo     = MCPToolInfo
	McpServerStatus = MCPServerStatus
)

// MCPServerConfig is the dial descriptor for a live MCP server.
type MCPServerConfig = lynxmcp.ServerConfig

// ErrUnknownMCPServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownMCPServer = errors.New("mcp: unknown server")

// MCPControl is the live-MCP-connections surface the engine exposes to
// workspace.mcp.* through the runtime.
type MCPControl interface {
	Statuses() []MCPServerStatus
	Tools(ctx context.Context, server string) ([]MCPToolInfo, error)
	Reconnect(ctx context.Context, name string) error
	Authorize(ctx context.Context, name string) error
	Probe(ctx context.Context, cfg MCPServerConfig) error
	Configure(ctx context.Context, cfg MCPServerConfig) error
	Remove(ctx context.Context, name string)
}
