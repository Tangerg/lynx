package server

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func (s *Server) mcpServerFromRequest(ctx context.Context, in protocol.ConfigureMCPServerRequest) (mcpserver.Server, error) {
	auth := in.Authorization
	if auth == "" && in.Name != "" {
		cur, ok, err := s.mcp.MCPRegisteredServer(ctx, in.Name)
		if err != nil {
			return mcpserver.Server{}, fmt.Errorf("load existing MCP server %q: %w", in.Name, err)
		}
		if ok {
			auth = cur.Authorization
		}
	}
	return mcpserver.Server{
		Name:             in.Name,
		Transport:        in.Transport,
		Enabled:          in.Enabled,
		Description:      in.Description,
		URL:              in.URL,
		Authorization:    auth,
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
