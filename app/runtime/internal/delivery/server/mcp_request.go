package server

import (
	"fmt"
	"time"

	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func mcpServerInputFromCandidate(in protocol.MCPServerCandidate) (mcpapp.ServerInput, error) {
	connection, err := mcpConnectionInputFromWire(in.Connection)
	if err != nil {
		return mcpapp.ServerInput{}, err
	}
	return mcpapp.ServerInput{
		Name:             in.Name,
		Enabled:          in.Enabled,
		Description:      in.Description,
		Connection:       connection,
		Timeout:          time.Duration(in.TimeoutSeconds) * time.Second,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
	}, nil
}

func mcpServerPatchFromRequest(in protocol.UpdateMCPServerRequest) (mcpapp.ServerPatch, error) {
	patch := mcpapp.ServerPatch{
		Enabled:          in.Enabled,
		Description:      in.Description,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
	}
	if in.Connection != nil {
		connection, err := mcpConnectionInputFromWire(*in.Connection)
		if err != nil {
			return mcpapp.ServerPatch{}, err
		}
		patch.Connection = &connection
	}
	if in.TimeoutSeconds != nil {
		timeout := time.Duration(*in.TimeoutSeconds) * time.Second
		patch.Timeout = &timeout
	}
	return patch, nil
}

func mcpConnectionInputFromWire(in protocol.MCPConnectionInput) (mcpapp.ConnectionInput, error) {
	transport, ok := mcpTransportFromWire(in.Type)
	if !ok {
		return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP transport %q", protocol.ErrInvalidParams, in.Type)
	}
	var authorization *mcpapp.AuthorizationChange
	if in.Authorization != nil {
		change := mcpapp.AuthorizationChange{Value: in.Authorization.Value}
		switch in.Authorization.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP authorization change %q", protocol.ErrInvalidParams, in.Authorization.Type)
		}
		authorization = &change
	}
	var headers *mcpapp.HeadersChange
	if in.Headers != nil {
		change := mcpapp.HeadersChange{Value: in.Headers.Value}
		switch in.Headers.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP headers change %q", protocol.ErrInvalidParams, in.Headers.Type)
		}
		headers = &change
	}
	var environment *mcpapp.EnvironmentChange
	if in.Env != nil {
		change := mcpapp.EnvironmentChange{Value: in.Env.Value}
		switch in.Env.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP environment change %q", protocol.ErrInvalidParams, in.Env.Type)
		}
		environment = &change
	}
	return mcpapp.ConnectionInput{
		Transport:     transport,
		URL:           in.URL,
		Authorization: authorization,
		Headers:       headers,
		Command:       in.Command,
		Args:          in.Args,
		Environment:   environment,
		Dir:           in.Dir,
	}, nil
}

func mcpTransportFromWire(transport protocol.MCPTransport) (mcpserver.Transport, bool) {
	switch transport {
	case protocol.MCPTransportStdio:
		return mcpserver.TransportStdio, true
	case protocol.MCPTransportStreamableHTTP:
		return mcpserver.TransportStreamableHTTP, true
	default:
		return "", false
	}
}
