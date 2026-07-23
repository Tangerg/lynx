package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// mcpServerCandidateFromRequest only decodes the transport request. Credential
// carry-forward is application policy because it relies on persisted state.
func mcpServerInputFromRequest(in protocol.ConfigureMCPServerRequest) integrations.MCPServerInput {
	return integrations.MCPServerInput{
		Name:             in.Name,
		Transport:        in.Transport,
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
	}
}
