package server

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func (s *Server) mcpServersWire(ctx context.Context) []protocol.McpServer {
	statuses := s.integrations.MCPServerStatuses(ctx)
	out := make([]protocol.McpServer, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, s.mcpServerWire(st))
	}
	return out
}

func (s *Server) mcpServerWire(st integrations.MCPServerStatus) protocol.McpServer {
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
	return protocol.McpServer{Name: st.Name, Status: status, ToolCount: st.ToolCount, Error: mcpProblemWire(st.Problem)}
}

func mcpStateWire(state integrations.MCPConnectionState) (protocol.McpStatus, bool) {
	switch state {
	case integrations.MCPConnecting:
		return protocol.McpConnecting, true
	case integrations.MCPConnected:
		return protocol.McpConnected, true
	case integrations.MCPFailed:
		return protocol.McpFailed, true
	case integrations.MCPNeedsAuth:
		return protocol.McpNeedsAuth, true
	default:
		return "", false
	}
}

func mcpProblemWire(problem *integrations.MCPProblem) *protocol.ProblemData {
	if problem == nil {
		return nil
	}
	return &protocol.ProblemData{Type: problem.Type, Detail: problem.Detail}
}

func mcpToolWire(t mcpserver.ToolInfo) protocol.McpTool {
	return protocol.McpTool{
		Server:      t.Server,
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema.Map(),
	}
}

func mcpConfigWire(srv integrations.MCPServerConfig) protocol.McpServerConfig {
	return protocol.McpServerConfig{
		Name:                srv.Name,
		Transport:           string(srv.Transport),
		Enabled:             srv.Enabled,
		Description:         srv.Description,
		URL:                 srv.URL,
		AuthorizationMasked: srv.AuthorizationMasked,
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
