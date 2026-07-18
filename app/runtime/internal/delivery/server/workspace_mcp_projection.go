package server

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func (s *Server) mcpServersWire(ctx context.Context) []protocol.McpServer {
	statuses := s.integrations.MCPServerStatuses()
	out := make([]protocol.McpServer, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, s.mcpServerWire(ctx, st))
	}
	return out
}

func (s *Server) mcpServerWire(ctx context.Context, st mcpserver.ConnectionStatus) protocol.McpServer {
	status, ok := mcpStateWire(st.State)
	if !ok {
		return protocol.McpServer{
			Name:   st.Name,
			Status: protocol.McpFailed,
			Error: &protocol.ProblemData{
				Type:   "mcp_invalid_connection_state",
				Detail: "runtime reported an unknown MCP connection state",
			},
		}
	}
	entry := protocol.McpServer{Name: st.Name, Status: status}
	switch st.State {
	case mcpserver.ConnectionConnected:
		entry.ToolCount = s.mcpToolCount(ctx, st.Name)
	case mcpserver.ConnectionFailed, mcpserver.ConnectionNeedsAuth:
		entry.Error = mcpDialFailedProblem(st.Err)
	}
	return entry
}

func mcpStateWire(state mcpserver.ConnectionState) (protocol.McpStatus, bool) {
	switch state {
	case mcpserver.ConnectionConnecting:
		return protocol.McpConnecting, true
	case mcpserver.ConnectionConnected:
		return protocol.McpConnected, true
	case mcpserver.ConnectionFailed:
		return protocol.McpFailed, true
	case mcpserver.ConnectionNeedsAuth:
		return protocol.McpNeedsAuth, true
	default:
		return "", false
	}
}

func (s *Server) mcpLiveStatus(ctx context.Context, name string) (protocol.McpStatus, *int, *protocol.ProblemData, bool) {
	st, ok := s.mcpStatusByName(name)
	if !ok {
		return "", nil, nil, false
	}
	wire := s.mcpServerWire(ctx, st)
	return wire.Status, wire.ToolCount, wire.Error, true
}

func (s *Server) mcpStatusByName(name string) (mcpserver.ConnectionStatus, bool) {
	for _, st := range s.integrations.MCPServerStatuses() {
		if st.Name == name {
			return st, true
		}
	}
	return mcpserver.ConnectionStatus{}, false
}

// mcpServerChangedEvent builds the settled mcp.serverChanged frame for name,
// reading its live status back from the pool (connected + tool count, or failed
// + reason). A name no longer tracked yields a bare frame (status omitted).
func (s *Server) mcpServerChangedEvent(ctx context.Context, server string) protocol.WorkspaceEvent {
	ev := protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server}
	if status, toolCount, problem, ok := s.mcpLiveStatus(ctx, server); ok {
		ev.Status, ev.ToolCount, ev.Error = status, toolCount, problem
	}
	return ev
}

func (s *Server) mcpToolCount(ctx context.Context, server string) *int {
	tools, err := s.integrations.MCPTools(ctx, server)
	if err != nil {
		return nil
	}
	count := len(tools)
	return &count
}

func mcpDialFailedProblem(err error) *protocol.ProblemData {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return &protocol.ProblemData{Type: "mcp_dial_failed", Detail: detail}
}

func mcpToolWire(t mcpserver.ToolInfo) protocol.McpTool {
	return protocol.McpTool{
		Server:      t.Server,
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema.Map(),
	}
}

func mcpConfigWire(srv mcpserver.Server) protocol.McpServerConfig {
	return protocol.McpServerConfig{
		Name:                srv.Name,
		Transport:           string(srv.Transport),
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
}
