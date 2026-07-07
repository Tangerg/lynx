package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// workspace.mcp.* — MCP is runtime-global, so these take no cwd (API.md §7.5).
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
	found, err := s.mcpTools.MCPTools(ctx, in.Server)
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
// The call validates the server name synchronously (unknown → invalid_params),
// publishes the connecting frame, then runs the dial off the request ctx
// (context.WithoutCancel) so a returning RPC doesn't abort it; the terminal
// frame is published when the dial settles.
func (s *Server) WorkspaceMCPReconnect(ctx context.Context, server string) error {
	return s.runMCPConnectionAction(ctx, server, s.mcpConnections.ReconnectMCPServer)
}

// WorkspaceMCPAuthorize starts the interactive OAuth sign-in for an HTTP MCP
// server (workspace.mcp.authorize). Like reconnect it is fire-and-forget: the
// name is validated synchronously (unknown → invalid_params), the connecting
// frame is published, then the flow — open the browser, catch the loopback
// redirect, exchange the code — runs off the request ctx (the human-in-the-loop
// flow far outlives one RPC) and the terminal mcp.serverChanged frame is
// published when it settles.
func (s *Server) WorkspaceMCPAuthorize(ctx context.Context, server string) error {
	return s.runMCPConnectionAction(ctx, server, s.mcpConnections.AuthorizeMCPServer)
}

// workspace.mcp registry CRUD — the editable configuration the settings pane
// drives. listConfigs returns the registry with bearer tokens masked;
// configure/remove/setEnabled persist + apply to the live connections, then
// publish mcp.serverChanged so the status view updates; test probes a
// candidate config without persisting.

// WorkspaceMCPListConfigs returns every registered MCP server's editable
// configuration (token masked). Live connection state is not included — read it
// from workspace.mcp.listServers (McpServer), keyed by name.
func (s *Server) WorkspaceMCPListConfigs(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServerConfig], error) {
	servers, err := s.mcpRegistryCatalog.ListMCPRegisteredServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpServerConfig, 0, len(servers))
	for _, srv := range servers {
		out = append(out, mcpConfigWire(srv))
	}
	return protocol.NewPage(out), nil
}

// WorkspaceMCPConfigure upserts a server in the registry and applies it to the
// live connections, returning the stored configuration (token masked). A blank
// Authorization preserves the existing server's token (see the request doc).
func (s *Server) WorkspaceMCPConfigure(ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpServerConfig, error) {
	if in.Name == "" {
		return nil, protocol.ErrInvalidParams
	}
	srv, err := s.mcpServerFromRequest(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := srv.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	if err := s.mcpRegistryMutations.ConfigureMCPServer(ctx, srv); err != nil {
		return nil, err
	}
	s.PublishWorkspaceEvent(s.mcpServerChangedEvent(ctx, in.Name))
	out := mcpConfigWire(srv)
	return &out, nil
}

// WorkspaceMCPRemove deletes a server from the registry + the live set. The
// follow-up mcp.serverChanged frame omits status (entry no longer exists).
func (s *Server) WorkspaceMCPRemove(ctx context.Context, name string) error {
	if name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.mcpRegistryMutations.RemoveMCPServer(ctx, name); err != nil {
		return err
	}
	s.PublishWorkspaceEvent(s.mcpServerChangedEvent(ctx, name))
	return nil
}

// WorkspaceMCPSetEnabled flips a server's enablement (enable → dial, disable →
// drop from the live set) and publishes the resulting status.
func (s *Server) WorkspaceMCPSetEnabled(ctx context.Context, in protocol.SetMCPEnabledRequest) error {
	if in.Name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.mcpRegistryMutations.SetMCPServerEnabled(ctx, in.Name, in.Enabled); err != nil {
		return err
	}
	s.PublishWorkspaceEvent(s.mcpServerChangedEvent(ctx, in.Name))
	return nil
}

// WorkspaceMCPTest probes a candidate configuration (a throwaway dial + tools
// list) without persisting — the connection-test button. A blank Authorization
// reuses the stored token, so testing an edit needn't re-enter the secret.
func (s *Server) WorkspaceMCPTest(ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpTestResult, error) {
	srv, err := s.mcpServerFromRequest(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := srv.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	if err := s.mcpRegistryProbe.TestMCPServer(ctx, srv); err != nil {
		return &protocol.McpTestResult{OK: false, Error: mcpDialFailedProblem(err)}, nil
	}
	return &protocol.McpTestResult{OK: true}, nil
}
