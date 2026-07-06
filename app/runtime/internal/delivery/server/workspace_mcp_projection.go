package server

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func (s *Server) mcpServersWire(ctx context.Context) []protocol.McpServer {
	statuses := s.mcp.MCPServerStatuses()
	out := make([]protocol.McpServer, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, s.mcpServerWire(ctx, st))
	}
	return out
}

func (s *Server) mcpServerWire(ctx context.Context, st kernel.McpServerStatus) protocol.McpServer {
	entry := protocol.McpServer{Name: st.Name, Status: protocol.McpStatus(st.Status)}
	switch st.Status {
	case "connected":
		entry.ToolCount = s.mcpToolCount(ctx, st.Name)
	case "failed":
		entry.Error = mcpDialFailedProblem(st.Err)
	}
	return entry
}

func (s *Server) mcpLiveStatus(ctx context.Context, name string) (protocol.McpStatus, *int, *protocol.ProblemData, bool) {
	st, ok := s.mcpStatusByName(name)
	if !ok {
		return "", nil, nil, false
	}
	wire := s.mcpServerWire(ctx, st)
	return wire.Status, wire.ToolCount, wire.Error, true
}

func (s *Server) mcpStatusByName(name string) (kernel.McpServerStatus, bool) {
	for _, st := range s.mcp.MCPServerStatuses() {
		if st.Name == name {
			return st, true
		}
	}
	return kernel.McpServerStatus{}, false
}

func (s *Server) mcpToolCount(ctx context.Context, server string) *int {
	tools, err := s.mcp.MCPTools(ctx, server)
	if err != nil {
		return nil
	}
	count := len(tools)
	return &count
}

func mcpDialFailedProblem(err error) *protocol.ProblemData {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return &protocol.ProblemData{Type: "mcp_dial_failed", Detail: detail}
}

func mcpToolWire(t kernel.McpToolInfo) protocol.McpTool {
	return protocol.McpTool{
		Server:      t.Server,
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
	}
}

func mcpConfigWire(srv mcpserver.Server) protocol.McpServerConfig {
	return protocol.McpServerConfig{
		Name:                srv.Name,
		Transport:           srv.Transport,
		Enabled:             srv.Enabled,
		Description:         srv.Description,
		URL:                 srv.URL,
		AuthorizationMasked: srv.MaskedAuthorization(),
		Headers:             srv.Headers,
		Command:             srv.Command,
		Args:                srv.Args,
		Env:                 srv.Env,
		Dir:                 srv.Dir,
		TimeoutSeconds:      int(srv.Timeout / time.Second),
		DisabledTools:       srv.DisabledTools,
		AutoApproveTools:    srv.AutoApproveTools,
	}
}
