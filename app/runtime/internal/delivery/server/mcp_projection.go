package server

import (
	"context"
	"fmt"
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
	return protocol.McpServer{
		Name: st.Name, Status: mcpStateWire(st.State),
		ToolCount: st.ToolCount, Error: mcpStatusProblem(st.State),
	}
}

// mcpStateWire panics on a connection state it does not map, matching the run
// presenter: an unmapped domain enum means this projection was not updated with
// the domain, which is a defect and not a state a server can be in. Reporting it
// as a failed server with an invented type shipped the defect as a user-visible
// verdict.
func mcpStateWire(state mcpserver.ConnectionState) protocol.McpStatus {
	switch state {
	case mcpserver.ConnectionConnecting:
		return protocol.McpConnecting
	case mcpserver.ConnectionConnected:
		return protocol.McpConnected
	case mcpserver.ConnectionFailed:
		return protocol.McpFailed
	case mcpserver.ConnectionNeedsAuth:
		return protocol.McpNeedsAuth
	default:
		panic("server: unknown MCP connection state")
	}
}

func mcpStatusProblem(state mcpserver.ConnectionState) *protocol.ProblemData {
	switch state {
	case mcpserver.ConnectionNeedsAuth:
		return &protocol.ProblemData{Type: protocol.ProblemMCPAuthorizationRequired}
	case mcpserver.ConnectionFailed:
		return &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}
	default:
		return nil
	}
}

func mcpProbeProblem() *protocol.ProblemData {
	return &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}
}

func mcpToolWire(t mcpserver.ToolInfo) protocol.McpTool {
	return protocol.McpTool{
		Server:      t.Server,
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema.Map(),
	}
}

func mcpConfigWire(srv integrations.MCPServerConfig) (protocol.McpServerConfig, error) {
	transport, ok := mcpTransportWire(srv.Transport)
	if !ok {
		return protocol.McpServerConfig{}, fmt.Errorf("mcp: unsupported transport %q", srv.Transport)
	}
	return protocol.McpServerConfig{
		Name:                srv.Name,
		Transport:           transport,
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
	}, nil
}

func mcpTransportWire(transport mcpserver.Transport) (protocol.McpTransport, bool) {
	switch transport {
	case mcpserver.TransportStdio:
		return protocol.McpTransportStdio, true
	case mcpserver.TransportStreamableHTTP:
		return protocol.McpTransportStreamableHTTP, true
	default:
		return "", false
	}
}
