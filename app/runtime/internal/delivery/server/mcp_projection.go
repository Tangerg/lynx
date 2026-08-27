package server

import (
	"fmt"
	"time"

	mcpapp "github.com/Tangerg/scope/app/runtime/internal/application/mcp"
	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

func presentMCPServer(server mcpapp.Server) (protocol.MCPServer, error) {
	connection, err := presentMCPConnection(server.Connection)
	if err != nil {
		return protocol.MCPServer{}, err
	}
	return protocol.MCPServer{
		Name:             server.Name,
		Description:      server.Description,
		Connection:       connection,
		TimeoutSeconds:   int(server.Timeout / time.Second),
		DisabledTools:    server.DisabledTools,
		AutoApproveTools: server.AutoApproveTools,
		Status:           presentMCPServerState(server.State),
	}, nil
}

func presentMCPConnection(connection mcpapp.Connection) (protocol.MCPConnection, error) {
	transport, ok := presentMCPTransport(connection.Transport)
	if !ok {
		return protocol.MCPConnection{}, fmt.Errorf("mcp: unsupported transport %q", connection.Transport)
	}
	return protocol.MCPConnection{
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

func presentMCPServerState(state mcpapp.ServerState) protocol.MCPServerState {
	out := protocol.MCPServerState{ToolCount: state.ToolCount}
	switch state.Type {
	case mcpapp.ServerDisabled:
		out.Type = protocol.MCPServerDisabled
	case mcpapp.ServerDisconnected:
		out.Type = protocol.MCPServerDisconnected
	case mcpapp.ServerConnecting:
		out.Type = protocol.MCPServerConnecting
	case mcpapp.ServerConnected:
		out.Type = protocol.MCPServerConnected
	case mcpapp.ServerFailed:
		out.Type = protocol.MCPServerFailed
		out.Error = mcpStatusProblem(mcpserver.ConnectionFailed)
	case mcpapp.ServerNeedsAuth:
		out.Type = protocol.MCPServerNeedsAuth
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

func presentMCPAuthorizationAttempt(attempt mcpapp.AuthorizationAttempt) protocol.MCPAuthorizationAttempt {
	status := protocol.MCPAuthorizationAttemptStatus{}
	switch attempt.Status {
	case mcpapp.AuthorizationAttemptPending:
		status.Type = protocol.MCPAuthorizationAttemptPending
	case mcpapp.AuthorizationAttemptSucceeded:
		status.Type = protocol.MCPAuthorizationAttemptSucceeded
	case mcpapp.AuthorizationAttemptFailed:
		status.Type = protocol.MCPAuthorizationAttemptFailed
		status.Error = &protocol.ProblemData{Type: protocol.ProblemMCPAuthorizationFailed}
	case mcpapp.AuthorizationAttemptCanceled:
		status.Type = protocol.MCPAuthorizationAttemptCanceled
	default:
		panic("server: unknown MCP authorization attempt status")
	}
	return protocol.MCPAuthorizationAttempt{
		ID: attempt.ID, Server: attempt.Server, Status: status,
		CreatedAt: attempt.CreatedAt, FinishedAt: attempt.FinishedAt,
	}
}

func presentMCPTool(tool mcpserver.AdvertisedTool) protocol.MCPTool {
	return protocol.MCPTool{
		Server:      tool.Server,
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema.Map(),
	}
}

func presentMCPTransport(transport mcpserver.Transport) (protocol.MCPTransport, bool) {
	switch transport {
	case mcpserver.TransportStdio:
		return protocol.MCPTransportStdio, true
	case mcpserver.TransportStreamableHTTP:
		return protocol.MCPTransportStreamableHTTP, true
	default:
		return "", false
	}
}
