package server

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func mcpServerInputFromCandidate(in protocol.MCPServerCandidate) (integrations.MCPServerInput, error) {
	connection, err := mcpConnectionInputFromWire(in.Connection)
	if err != nil {
		return integrations.MCPServerInput{}, err
	}
	return integrations.MCPServerInput{
		Name:             in.Name,
		Enabled:          in.Enabled,
		Description:      in.Description,
		Connection:       connection,
		Timeout:          time.Duration(in.TimeoutSeconds) * time.Second,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
	}, nil
}

func mcpServerPatchFromRequest(in protocol.UpdateMCPServerRequest) (integrations.MCPServerPatch, error) {
	patch := integrations.MCPServerPatch{
		Enabled:          in.Enabled,
		Description:      in.Description,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
	}
	if in.Connection != nil {
		connection, err := mcpConnectionInputFromWire(*in.Connection)
		if err != nil {
			return integrations.MCPServerPatch{}, err
		}
		patch.Connection = &connection
	}
	if in.TimeoutSeconds != nil {
		timeout := time.Duration(*in.TimeoutSeconds) * time.Second
		patch.Timeout = &timeout
	}
	return patch, nil
}

func mcpConnectionInputFromWire(in protocol.McpConnectionInput) (integrations.MCPConnectionInput, error) {
	transport, ok := mcpTransportFromWire(in.Type)
	if !ok {
		return integrations.MCPConnectionInput{}, fmt.Errorf("%w: unknown MCP transport %q", protocol.ErrInvalidParams, in.Type)
	}
	var authorization *integrations.MCPAuthorizationChange
	if in.Authorization != nil {
		change := integrations.MCPAuthorizationChange{Value: in.Authorization.Value}
		switch in.Authorization.Type {
		case protocol.McpSecretSet:
			change.Kind = integrations.MCPSecretSet
		case protocol.McpSecretClear:
			change.Kind = integrations.MCPSecretClear
		default:
			return integrations.MCPConnectionInput{}, fmt.Errorf("%w: unknown MCP authorization change %q", protocol.ErrInvalidParams, in.Authorization.Type)
		}
		authorization = &change
	}
	var headers *integrations.MCPHeadersChange
	if in.Headers != nil {
		change := integrations.MCPHeadersChange{Value: in.Headers.Value}
		switch in.Headers.Type {
		case protocol.McpSecretSet:
			change.Kind = integrations.MCPSecretSet
		case protocol.McpSecretClear:
			change.Kind = integrations.MCPSecretClear
		default:
			return integrations.MCPConnectionInput{}, fmt.Errorf("%w: unknown MCP headers change %q", protocol.ErrInvalidParams, in.Headers.Type)
		}
		headers = &change
	}
	var environment *integrations.MCPEnvironmentChange
	if in.Env != nil {
		change := integrations.MCPEnvironmentChange{Value: in.Env.Value}
		switch in.Env.Type {
		case protocol.McpSecretSet:
			change.Kind = integrations.MCPSecretSet
		case protocol.McpSecretClear:
			change.Kind = integrations.MCPSecretClear
		default:
			return integrations.MCPConnectionInput{}, fmt.Errorf("%w: unknown MCP environment change %q", protocol.ErrInvalidParams, in.Env.Type)
		}
		environment = &change
	}
	return integrations.MCPConnectionInput{
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

func mcpTransportFromWire(transport protocol.McpTransport) (mcpserver.Transport, bool) {
	switch transport {
	case protocol.McpTransportStdio:
		return mcpserver.TransportStdio, true
	case protocol.McpTransportStreamableHTTP:
		return mcpserver.TransportStreamableHTTP, true
	default:
		return "", false
	}
}
