package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// mcp.* is runtime-global, so these methods take no cwd (API.md §7.5).
// Reconnect's outcome rides the workspace event stream as mcp.serverChanged
// (AUX_API §5).

// WorkspaceMCPListServers lists every configured MCP server with its real
// connection state (AUX_API §5.1). Boot tolerates a per-server failure, so the
// list includes servers that couldn't connect — each carrying its failure
// reason as Error — alongside the connected ones, instead of the old
// "everything is connected" assumption.
func (s *Server) WorkspaceMCPListServers(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
	return protocol.NewPage(s.mcpServersWire(ctx)), nil
}

// WorkspaceMCPListTools lists tools advertised by the connected MCP servers,
// scoped to in.Server when set (empty = all). Each tool's bare name + the
// server it belongs to are kept separate on the wire (the model sees them as
// "<server>_<name>").
func (s *Server) WorkspaceMCPListTools(ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
	found, err := s.integrations.MCPTools(ctx, in.Server)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpTool, 0, len(found))
	for _, t := range found {
		out = append(out, mcpToolWire(t))
	}
	return protocol.NewPage(out), nil
}

// WorkspaceMCPReconnect re-dials a configured MCP server (AUX_API §5.2). It has
// no synchronous result — the outcome rides notifications.workspace.event as
// mcp.serverChanged, in the guaranteed order connecting → (connected | failed).
// The capabilities coordinator validates the name synchronously (unknown →
// invalid_params) then runs the dial fire-and-forget on its component task group,
// publishing the connecting + settled frames through the MCP-status bridge.
func (s *Server) WorkspaceMCPReconnect(ctx context.Context, server string) error {
	return wireMCPError(s.integrations.ReconnectMCPServer(ctx, server))
}

// WorkspaceMCPAuthorize starts the interactive OAuth sign-in for an HTTP MCP
// server (mcp.servers.authorize). Like reconnect it is fire-and-forget: the
// coordinator validates the name synchronously (unknown → invalid_params) then
// runs the flow — open the browser, catch the loopback redirect, exchange the
// code — publishing the connecting + settled frames as it settles.
func (s *Server) WorkspaceMCPAuthorize(ctx context.Context, server string) error {
	return wireMCPError(s.integrations.AuthorizeMCPServer(ctx, server))
}

// wireMCPError maps the coordinator's unknown-server sentinel onto invalid_params
// (an unknown / empty name is a bad request); every other error surfaces as-is.
func wireMCPError(err error) error {
	if errors.Is(err, integrations.ErrUnknownMCPServer) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	if errors.Is(err, integrations.ErrInvalidMCPServerConfiguration) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

// observeMCPStatus installs the MCP-status bridge consumer: the coordinator
// publishes a connection transition, and the Server maps it to a workspace event
// on the hub. connecting=true is the transient pre-frame; connecting=false reads
// back the settled live status (connected + tool count, or failed + reason).
func (s *Server) observeMCPStatus(src MCPStatusSource) {
	src.Observe(func(status integrations.MCPServerStatus) {
		event := protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: status.Name}
		if status.Known {
			wire := s.mcpServerWire(status)
			event.Status, event.ToolCount, event.Error = wire.Status, wire.ToolCount, wire.Error
		}
		s.PublishWorkspaceEvent(event)
	})
}

// mcp.configs registry CRUD — the editable configuration the settings pane
// drives. listConfigs returns the registry with bearer tokens masked;
// configure/remove/setEnabled persist + apply to the live connections, while
// the application status bridge publishes mcp.serverChanged. Test probes a
// candidate config without persisting.

// WorkspaceMCPListConfigs returns every registered MCP server's editable
// configuration (token masked). Live connection state is not included — read it
// from mcp.servers.list (McpServer), keyed by name.
func (s *Server) WorkspaceMCPListConfigs(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServerConfig], error) {
	servers, err := s.integrations.ListMCPServerConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpServerConfig, 0, len(servers))
	for _, config := range servers {
		out = append(out, mcpConfigWire(config))
	}
	return protocol.NewPage(out), nil
}

// WorkspaceMCPConfigure upserts a server in the registry and applies it to the
// live connections, returning the stored configuration (token masked). A blank
// Authorization preserves the existing token only for the same HTTP origin.
func (s *Server) WorkspaceMCPConfigure(ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpServerConfig, error) {
	if in.Name == "" {
		return nil, protocol.ErrInvalidParams
	}
	config, err := s.integrations.ConfigureMCPServer(ctx, mcpServerInputFromRequest(in))
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := mcpConfigWire(config)
	return &out, nil
}

// WorkspaceMCPRemove deletes a server from the registry + the live set. The
// follow-up mcp.serverChanged frame omits status (entry no longer exists).
func (s *Server) WorkspaceMCPRemove(ctx context.Context, name string) error {
	if name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.integrations.RemoveMCPServer(ctx, name); err != nil {
		return err
	}
	return nil
}

// WorkspaceMCPSetEnabled flips a server's enablement (enable → dial, disable →
// drop from the live set) and publishes the resulting status.
func (s *Server) WorkspaceMCPSetEnabled(ctx context.Context, in protocol.SetMCPEnabledRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.integrations.SetMCPServerEnabled(ctx, in.Name, in.Enabled); err != nil {
		return err
	}
	return nil
}

// WorkspaceMCPTest probes a candidate configuration (a throwaway dial + tools
// list) without persisting — the connection-test button. A blank Authorization
// reuses the stored token only for the same HTTP origin, so testing an ordinary
// edit needn't re-enter the secret and testing a new endpoint cannot leak it.
func (s *Server) WorkspaceMCPTest(ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpTestResult, error) {
	result, err := s.integrations.TestMCPServer(ctx, mcpServerInputFromRequest(in))
	if err != nil {
		return nil, wireMCPError(err)
	}
	return &protocol.McpTestResult{OK: result.OK, Error: mcpProblemWire(result.Problem)}, nil
}
