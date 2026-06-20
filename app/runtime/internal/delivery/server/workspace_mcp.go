package server

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
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
	statuses := s.rt.MCPServerStatuses()
	out := make([]protocol.McpServer, 0, len(statuses))
	for _, st := range statuses {
		entry := protocol.McpServer{Name: st.Name, Status: protocol.McpStatus(st.Status)}
		switch st.Status {
		case "connected":
			// Inline the tool count so the client needn't ⨝ listTools (AUX_API §5.1).
			if tools, err := s.rt.MCPTools(ctx, st.Name); err == nil {
				count := len(tools)
				entry.ToolCount = &count
			}
		case "failed":
			entry.Error = mcpDialFailedProblem(st.Err)
		}
		out = append(out, entry)
	}
	return protocol.NewPage(out), nil
}

// mcpDialFailedProblem renders a failed MCP server's reason for the wire
// (McpServer.error / mcp.serverChanged error). A nil err yields an empty detail.
func mcpDialFailedProblem(err error) *protocol.ProblemData {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	// No Channel: an MCP dial failure surfaces via a workspace event, not over
	// one of the three delivery channels (rpc/run/tool), so it stays
	// unclassified rather than inventing a 4th ErrorChannel value.
	return &protocol.ProblemData{Type: "mcp_dial_failed", Detail: detail}
}

// WorkspaceMCPListTools lists tools advertised by the connected MCP servers,
// scoped to in.Server when set (empty = all). Each tool's bare name + the
// server it belongs to are kept separate on the wire (the model sees them as
// "<server>_<name>").
func (s *Server) WorkspaceMCPListTools(ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
	found, err := s.rt.MCPTools(ctx, in.Server)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpTool, 0, len(found))
	for _, t := range found {
		out = append(out, protocol.McpTool{
			Server:      t.Server,
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
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
	if server == "" {
		return protocol.ErrInvalidParams
	}
	known := false
	for _, st := range s.rt.MCPServerStatuses() {
		if st.Name == server {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("%w: unknown MCP server %q", protocol.ErrInvalidParams, server)
	}

	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server, Status: protocol.McpConnecting})
	bg := context.WithoutCancel(ctx)
	go func() {
		_ = s.rt.ReconnectMCPServer(bg, server) // terminal status is read back below
		s.PublishWorkspaceEvent(s.mcpServerChangedEvent(bg, server))
	}()
	return nil
}

// mcpServerChangedEvent builds the terminal mcp.serverChanged frame from a
// server's current state: connected inlines its tool count, failed carries its
// reason as Error, and a server that vanished from config omits status (AUX_API
// §5.2: "status omitted = entry no longer exists").
func (s *Server) mcpServerChangedEvent(ctx context.Context, server string) protocol.WorkspaceEvent {
	ev := protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server}
	for _, st := range s.rt.MCPServerStatuses() {
		if st.Name != server {
			continue
		}
		ev.Status = protocol.McpStatus(st.Status)
		switch st.Status {
		case "connected":
			if tools, err := s.rt.MCPTools(ctx, server); err == nil {
				count := len(tools)
				ev.ToolCount = &count
			}
		case "failed":
			ev.Error = mcpDialFailedProblem(st.Err)
		}
		break
	}
	return ev
}

// workspace.mcp registry CRUD — the editable configuration the settings pane
// drives. listConfigs returns the registry (with the bearer token masked and
// the best-effort live status); configure/remove/setEnabled persist + apply to
// the live connections (then publish mcp.serverChanged so the status view
// updates); test probes a candidate config without persisting.

// WorkspaceMCPListConfigs returns every registered MCP server's editable
// configuration, each annotated with its best-effort live connection status.
func (s *Server) WorkspaceMCPListConfigs(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServerConfig], error) {
	servers, err := s.rt.MCPRegistry().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpServerConfig, 0, len(servers))
	for _, srv := range servers {
		out = append(out, s.mcpConfigWire(ctx, srv))
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
	srv := s.mcpServerFromRequest(ctx, in)
	if err := srv.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := s.rt.ConfigureMCPServer(ctx, srv); err != nil {
		return nil, err
	}
	s.PublishWorkspaceEvent(s.mcpServerChangedEvent(ctx, in.Name))
	out := s.mcpConfigWire(ctx, srv)
	return &out, nil
}

// WorkspaceMCPRemove deletes a server from the registry + the live set. The
// follow-up mcp.serverChanged frame omits status (entry no longer exists).
func (s *Server) WorkspaceMCPRemove(ctx context.Context, name string) error {
	if name == "" {
		return protocol.ErrInvalidParams
	}
	if err := s.rt.RemoveMCPServer(ctx, name); err != nil {
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
	if err := s.rt.SetMCPServerEnabled(ctx, in.Name, in.Enabled); err != nil {
		return err
	}
	s.PublishWorkspaceEvent(s.mcpServerChangedEvent(ctx, in.Name))
	return nil
}

// WorkspaceMCPTest probes a candidate configuration (a throwaway dial + tools
// list) without persisting — the connection-test button. A blank Authorization
// reuses the stored token, so testing an edit needn't re-enter the secret.
func (s *Server) WorkspaceMCPTest(ctx context.Context, in protocol.ConfigureMCPServerRequest) (*protocol.McpTestResult, error) {
	srv := s.mcpServerFromRequest(ctx, in)
	if err := srv.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := s.rt.TestMCPServer(ctx, srv); err != nil {
		return &protocol.McpTestResult{OK: false, Error: mcpDialFailedProblem(err)}, nil
	}
	return &protocol.McpTestResult{OK: true}, nil
}

// mcpServerFromRequest maps a configure/test request to a registry entry,
// preserving the existing stored token when Authorization is blank.
func (s *Server) mcpServerFromRequest(ctx context.Context, in protocol.ConfigureMCPServerRequest) mcpserver.Server {
	auth := in.Authorization
	if auth == "" {
		if cur, ok, err := s.rt.MCPRegistry().Get(ctx, in.Name); err == nil && ok {
			auth = cur.Authorization
		}
	}
	return mcpserver.Server{
		Name:             in.Name,
		Transport:        in.Transport,
		Enabled:          in.Enabled,
		Description:      in.Description,
		URL:              in.URL,
		Authorization:    auth,
		Headers:          in.Headers,
		Command:          in.Command,
		Args:             in.Args,
		Env:              in.Env,
		Dir:              in.Dir,
		Timeout:          time.Duration(in.TimeoutSeconds) * time.Second,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
	}
}

// mcpConfigWire projects a registry entry to the wire (token masked) and
// annotates it with the best-effort live connection status (only meaningful for
// an enabled, dialed server).
func (s *Server) mcpConfigWire(ctx context.Context, srv mcpserver.Server) protocol.McpServerConfig {
	out := protocol.McpServerConfig{
		Name:                srv.Name,
		Transport:           srv.Transport,
		Enabled:             srv.Enabled,
		Description:         srv.Description,
		URL:                 srv.URL,
		AuthorizationMasked: srv.MaskedAuthorization(),
		Headers:             srv.Headers,
		Command:             srv.Command,
		Args:                srv.Args,
		Env:                 srv.Env,
		Dir:                 srv.Dir,
		TimeoutSeconds:      int(srv.Timeout / time.Second),
		DisabledTools:       srv.DisabledTools,
		AutoApproveTools:    srv.AutoApproveTools,
	}
	if !srv.Enabled {
		return out
	}
	for _, st := range s.rt.MCPServerStatuses() {
		if st.Name != srv.Name {
			continue
		}
		out.Status = protocol.McpStatus(st.Status)
		switch st.Status {
		case "connected":
			if tools, err := s.rt.MCPTools(ctx, srv.Name); err == nil {
				count := len(tools)
				out.ToolCount = &count
			}
		case "failed":
			out.Error = mcpDialFailedProblem(st.Err)
		}
		break
	}
	return out
}
