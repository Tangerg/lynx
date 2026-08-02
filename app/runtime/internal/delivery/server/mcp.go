package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// mcp.* is runtime-global, so these methods take no workspace reference.

// ListMCPServers returns the single authoritative MCP resource collection:
// durable configuration enriched with current live state.
func (s *Server) ListMCPServers(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
	servers, err := s.integrations.MCPServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpServer, 0, len(servers))
	for _, server := range servers {
		wire, err := mcpServerWire(server)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return protocol.NewPage(out), nil
}

// CreateMCPServer creates and returns one unified MCP server resource.
func (s *Server) CreateMCPServer(ctx context.Context, in protocol.CreateMCPServerRequest) (*protocol.McpServer, error) {
	input, err := mcpServerInputFromCandidate(in)
	if err != nil {
		return nil, err
	}
	server, err := s.integrations.CreateMCPServer(ctx, input)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := mcpServerWire(server)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMCPServer applies an explicit partial update and returns the resulting
// unified resource.
func (s *Server) UpdateMCPServer(ctx context.Context, in protocol.UpdateMCPServerRequest) (*protocol.McpServer, error) {
	patch, err := mcpServerPatchFromRequest(in)
	if err != nil {
		return nil, err
	}
	server, err := s.integrations.UpdateMCPServer(ctx, in.Server, patch)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out, err := mcpServerWire(server)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMCPServer deletes one configured server and its live projection.
func (s *Server) DeleteMCPServer(ctx context.Context, server string) error {
	return wireMCPError(s.integrations.DeleteMCPServer(ctx, server))
}

// TestMCPServer probes a complete candidate without persisting it.
func (s *Server) TestMCPServer(ctx context.Context, in protocol.MCPServerCandidate) (*protocol.McpTestResult, error) {
	input, err := mcpServerInputFromCandidate(in)
	if err != nil {
		return nil, err
	}
	result, err := s.integrations.TestMCPServer(ctx, input)
	if err != nil {
		return nil, wireMCPError(err)
	}
	var problem *protocol.ProblemData
	if !result.OK {
		problem = mcpProbeProblem()
	}
	return &protocol.McpTestResult{OK: result.OK, Error: problem}, nil
}

// ListMCPTools lists tools advertised by connected MCP servers, optionally
// narrowed to one server.
func (s *Server) ListMCPTools(ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
	found, err := s.integrations.MCPTools(ctx, in.Server)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.McpTool, 0, len(found))
	for _, tool := range found {
		out = append(out, mcpToolWire(tool))
	}
	return protocol.NewPage(out), nil
}

// ReconnectMCPServer starts a new live dial. Its state transitions invalidate
// the server resource through runtime.event.
func (s *Server) ReconnectMCPServer(ctx context.Context, server string) error {
	return wireMCPError(s.integrations.ReconnectMCPServer(ctx, server))
}

// CreateMCPAuthorizationAttempt starts interactive OAuth and returns its
// observable asynchronous resource immediately.
func (s *Server) CreateMCPAuthorizationAttempt(ctx context.Context, server string) (*protocol.McpAuthorizationAttempt, error) {
	attempt, err := s.integrations.CreateMCPAuthorizationAttempt(ctx, server)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := mcpAuthorizationAttemptWire(attempt)
	return &out, nil
}

// GetMCPAuthorizationAttempt returns a pending or retained terminal OAuth flow.
func (s *Server) GetMCPAuthorizationAttempt(ctx context.Context, attemptID string) (*protocol.McpAuthorizationAttempt, error) {
	attempt, err := s.integrations.MCPAuthorizationAttempt(ctx, attemptID)
	if err != nil {
		return nil, wireMCPError(err)
	}
	out := mcpAuthorizationAttemptWire(attempt)
	return &out, nil
}

func wireMCPError(err error) error {
	switch {
	case errors.Is(err, integrations.ErrUnknownMCPServer):
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerNotFound, err)
	case errors.Is(err, integrations.ErrMCPServerAlreadyExists):
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerAlreadyExists, err)
	case errors.Is(err, integrations.ErrMCPServerDisabled):
		return fmt.Errorf("%w: %w", protocol.ErrMCPServerDisabled, err)
	case errors.Is(err, integrations.ErrMCPAuthorizationAttemptNotFound):
		return fmt.Errorf("%w: %w", protocol.ErrMCPAuthorizationAttemptNotFound, err)
	case errors.Is(err, integrations.ErrMCPAuthorizationUnsupported):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, integrations.ErrInvalidMCPServerConfiguration):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

// observeMCPStatus publishes invalidations only. The resource value remains
// authoritative in mcp.servers.list.
func (s *Server) observeMCPStatus(src Source[integrations.MCPServerStatus]) {
	src.Observe(func(status integrations.MCPServerStatus) {
		s.PublishRuntimeEvent(protocol.RuntimeEvent{
			Type: protocol.RuntimeMCPChanged, ServerIDs: []string{status.Name},
		})
	})
}
