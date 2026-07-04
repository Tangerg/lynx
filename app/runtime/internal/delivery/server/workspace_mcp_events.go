package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

type mcpConnectionAction func(context.Context, string) error

func (s *Server) runMCPConnectionAction(ctx context.Context, server string, action mcpConnectionAction) error {
	if server == "" {
		return protocol.ErrInvalidParams
	}
	if _, ok := s.mcpStatusByName(server); !ok {
		return fmt.Errorf("%w: unknown MCP server %q", protocol.ErrInvalidParams, server)
	}

	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server, Status: protocol.McpConnecting})
	bg := context.WithoutCancel(ctx)
	go func() {
		_ = action(bg, server)
		s.PublishWorkspaceEvent(s.mcpServerChangedEvent(bg, server))
	}()
	return nil
}

func (s *Server) mcpServerChangedEvent(ctx context.Context, server string) protocol.WorkspaceEvent {
	ev := protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server}
	if status, toolCount, problem, ok := s.mcpLiveStatus(ctx, server); ok {
		ev.Status, ev.ToolCount, ev.Error = status, toolCount, problem
	}
	return ev
}
