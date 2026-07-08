package toolport

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
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

// MCPTransport names the live MCP connection transport at the kernel port.
type MCPTransport string

const (
	MCPTransportHTTP  MCPTransport = "http"
	MCPTransportStdio MCPTransport = "stdio"
)

// MCPServerConfig is the live MCP server descriptor accepted by the kernel
// port. The concrete MCP adapter maps it to its own dial config at the infra
// boundary.
type MCPServerConfig struct {
	Name          string
	Transport     MCPTransport
	Endpoint      string
	Command       string
	Args          []string
	Env           []string
	Dir           string
	Authorization string
	Headers       map[string]string
	Timeout       time.Duration
}

// ErrUnknownMCPServer is returned when a live MCP operation addresses a server
// that was never configured.
var ErrUnknownMCPServer = errors.New("mcp: unknown server")

// MCPStatusReader reads live status for configured MCP servers.
type MCPStatusReader interface {
	Statuses() []MCPServerStatus
}

// MCPToolCatalog lists tools advertised by live MCP server connections.
type MCPToolCatalog interface {
	Tools(ctx context.Context, server string) ([]MCPToolInfo, error)
}

// MCPConnectionCommands operates on an already configured MCP server's live
// connection.
type MCPConnectionCommands interface {
	Reconnect(ctx context.Context, name string) error
	Authorize(ctx context.Context, name string) error
}

// MCPRegistryCommands probes and applies live MCP server registry changes.
type MCPRegistryCommands interface {
	Probe(ctx context.Context, cfg MCPServerConfig) error
	Configure(ctx context.Context, cfg MCPServerConfig) error
	Remove(ctx context.Context, name string)
}
