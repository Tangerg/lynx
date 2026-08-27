package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListMCPServers returns configured MCP servers.
func (r *Runtime) ListMCPServers(ctx context.Context, options CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.MCPServer]](ctx, operation.MCPServersList, struct{}{}, callOptions(options))
}

// CreateMCPServer creates an MCP server configuration.
func (r *Runtime) CreateMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CommandOptions) (*protocol.MCPServer, error) {
	return r.invoke[protocol.MCPServerCandidate, *protocol.MCPServer](ctx, operation.MCPServersCreate, request, commandOptions(options))
}

// UpdateMCPServer updates an MCP server configuration.
func (r *Runtime) UpdateMCPServer(ctx context.Context, request protocol.UpdateMCPServerRequest, options CommandOptions) (*protocol.MCPServer, error) {
	return r.invoke[protocol.UpdateMCPServerRequest, *protocol.MCPServer](ctx, operation.MCPServersUpdate, request, commandOptions(options))
}

// DeleteMCPServer deletes an MCP server configuration.
func (r *Runtime) DeleteMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.MCPServersDelete, request, commandOptions(options))
}

// TestMCPServer probes an MCP server candidate without persisting it.
func (r *Runtime) TestMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CallOptions) (*protocol.MCPTestResult, error) {
	return r.invoke[protocol.MCPServerCandidate, *protocol.MCPTestResult](ctx, operation.MCPServersTest, request, callOptions(options))
}

// ListMCPTools returns tools advertised by configured MCP servers.
func (r *Runtime) ListMCPTools(ctx context.Context, request protocol.MCPListToolsRequest, options CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	return r.invoke[protocol.MCPListToolsRequest, *protocol.Page[protocol.MCPTool]](ctx, operation.MCPToolsList, request, callOptions(options))
}

// ReconnectMCPServer reconnects one enabled MCP server.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.MCPServersReconnect, request, commandOptions(options))
}

// CreateMCPAuthorizationAttempt starts browser authorization for an MCP server.
func (r *Runtime) CreateMCPAuthorizationAttempt(ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return r.invoke[protocol.CreateMCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, operation.MCPAuthorizationAttemptsCreate, request, commandOptions(options))
}

// GetMCPAuthorizationAttempt returns one MCP authorization attempt.
func (r *Runtime) GetMCPAuthorizationAttempt(ctx context.Context, request protocol.MCPAuthorizationAttemptRequest, options CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return r.invoke[protocol.MCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, operation.MCPAuthorizationAttemptsGet, request, callOptions(options))
}
