package server

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// mcpServerCandidateFromRequest only decodes the transport request. Credential
// carry-forward is application policy because it relies on persisted state.
func mcpServerCandidateFromRequest(in protocol.ConfigureMCPServerRequest) mcpserver.Server {
	return mcpserver.Server{
		Name:             in.Name,
		Transport:        mcpserver.Transport(in.Transport),
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
