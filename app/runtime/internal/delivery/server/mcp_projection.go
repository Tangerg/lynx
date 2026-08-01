package server

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func mcpServerWire(server integrations.MCPServer) (protocol.McpServer, error) {
	connection, err := mcpConnectionWire(server.Connection)
	if err != nil {
		return protocol.McpServer{}, err
	}
	return protocol.McpServer{
		Name:             server.Name,
		Description:      server.Description,
		Connection:       connection,
		TimeoutSeconds:   int(server.Timeout / time.Second),
		DisabledTools:    server.DisabledTools,
		AutoApproveTools: server.AutoApproveTools,
		Status:           mcpServerStateWire(server.State),
	}, nil
}

func mcpConnectionWire(connection integrations.MCPConnection) (protocol.McpConnection, error) {
	transport, ok := mcpTransportWire(connection.Transport)
	if !ok {
		return protocol.McpConnection{}, fmt.Errorf("mcp: unsupported transport %q", connection.Transport)
	}
	return protocol.McpConnection{
		Type:                transport,
		URL:                 connection.URL,
		AuthorizationMasked: connection.AuthorizationMasked,
		HeadersMasked:       connection.HeadersMasked,
		Command:             connection.Command,
		Args:                connection.Args,
		EnvMasked:           connection.EnvironmentMasked,
		Dir:                 connection.Dir,
	}, nil
}

func mcpServerStateWire(state integrations.MCPServerState) protocol.McpServerState {
	out := protocol.McpServerState{ToolCount: state.ToolCount}
	switch state.Type {
	case integrations.MCPServerDisabled:
		out.Type = protocol.McpServerDisabled
	case integrations.MCPServerDisconnected:
		out.Type = protocol.McpServerDisconnected
	case integrations.MCPServerConnecting:
		out.Type = protocol.McpServerConnecting
	case integrations.MCPServerConnected:
		out.Type = protocol.McpServerConnected
	case integrations.MCPServerFailed:
		out.Type = protocol.McpServerFailed
		out.Error = mcpStatusProblem(mcpserver.ConnectionFailed)
	case integrations.MCPServerNeedsAuth:
		out.Type = protocol.McpServerNeedsAuth
		out.Error = mcpStatusProblem(mcpserver.ConnectionNeedsAuth)
	default:
		panic("server: unknown MCP server state")
	}
	return out
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

func mcpToolWire(tool mcpserver.ToolInfo) protocol.McpTool {
	return protocol.McpTool{
		Server:      tool.Server,
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema.Map(),
	}
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
