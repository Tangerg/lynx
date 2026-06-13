package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
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
	return &protocol.ProblemData{Type: "mcp_dial_failed", Channel: "mcp", Detail: detail}
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

	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: "mcp.serverChanged", Server: server, Status: "connecting"})
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
	ev := protocol.WorkspaceEvent{Type: "mcp.serverChanged", Server: server}
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
