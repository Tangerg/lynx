package server

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// mcpServerCandidateFromRequest only decodes the transport request. Credential
// carry-forward is application policy because it relies on persisted state.
func mcpServerInputFromRequest(in protocol.ConfigureMCPServerRequest) (integrations.MCPServerInput, error) {
	transport, ok := mcpTransportFromWire(in.Transport)
	if !ok {
		return integrations.MCPServerInput{}, fmt.Errorf("%w: unknown MCP transport %q", protocol.ErrInvalidParams, in.Transport)
	}
	return integrations.MCPServerInput{
		Name:             in.Name,
		Transport:        transport,
		Enabled:          in.Enabled,
		Description:      in.Description,
		URL:              in.URL,
		Authorization:    in.Authorization,
		Headers:          in.Headers,
		Command:          in.Command,
		Args:             in.Args,
		Env:              in.Env,
		Dir:              in.Dir,
		Timeout:          time.Duration(in.TimeoutSeconds) * time.Second,
		DisabledTools:    in.DisabledTools,
		AutoApproveTools: in.AutoApproveTools,
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
