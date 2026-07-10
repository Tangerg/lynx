package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

type mcpConnectionAction func(context.Context, string) error

var errServerClosed = errors.New("server: closed")

func (s *Server) runMCPConnectionAction(ctx context.Context, server string, action mcpConnectionAction) error {
	if server == "" {
		return protocol.ErrInvalidParams
	}
	if _, ok := s.mcpStatusByName(server); !ok {
		return fmt.Errorf("%w: unknown MCP server %q", protocol.ErrInvalidParams, server)
	}

	if !s.tasks.Start(ctx, func(bg context.Context) {
		s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server, Status: protocol.McpConnecting})
		_ = action(bg, server)
		s.PublishWorkspaceEvent(s.mcpServerChangedEvent(bg, server))
	}) {
		return errServerClosed
	}
	return nil
}

func (s *Server) mcpServerChangedEvent(ctx context.Context, server string) protocol.WorkspaceEvent {
	ev := protocol.WorkspaceEvent{Type: protocol.WorkspaceEventMCPServerChanged, Server: server}
	if status, toolCount, problem, ok := s.mcpLiveStatus(ctx, server); ok {
		ev.Status, ev.ToolCount, ev.Error = status, toolCount, problem
	}
	return ev
}
