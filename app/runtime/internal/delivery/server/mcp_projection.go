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
	return protocol.McpServer{Name: st.Name, Status: status, ToolCount: st.ToolCount, Error: mcpStatusProblem(st.State)}
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

func mcpStatusProblem(state mcpserver.ConnectionState) *protocol.ProblemData {
	switch state {
	case mcpserver.ConnectionNeedsAuth:
		return &protocol.ProblemData{Type: "mcp_authorization_required", Detail: "MCP authorization is required."}
	case mcpserver.ConnectionFailed:
		return &protocol.ProblemData{Type: "mcp_dial_failed", Detail: "MCP connection failed."}
	default:
		return nil
	}
}

func mcpProbeProblem() *protocol.ProblemData {
	return &protocol.ProblemData{Type: "mcp_dial_failed", Detail: "MCP connection test failed."}
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
