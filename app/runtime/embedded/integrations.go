package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListMCPServers returns configured MCP servers.
func (r *Runtime) ListMCPServers(ctx context.Context, options CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	return invoke[struct{}, *protocol.Page[protocol.MCPServer]](ctx, r, "mcp.servers.list", struct{}{}, callOptions(options))
}

// CreateMCPServer creates an MCP server configuration.
func (r *Runtime) CreateMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CommandOptions) (*protocol.MCPServer, error) {
	return invoke[protocol.MCPServerCandidate, *protocol.MCPServer](ctx, r, "mcp.servers.create", request, commandOptions(options))
}

// UpdateMCPServer updates an MCP server configuration.
func (r *Runtime) UpdateMCPServer(ctx context.Context, request protocol.UpdateMCPServerRequest, options CommandOptions) (*protocol.MCPServer, error) {
	return invoke[protocol.UpdateMCPServerRequest, *protocol.MCPServer](ctx, r, "mcp.servers.update", request, commandOptions(options))
}

// DeleteMCPServer deletes an MCP server configuration.
func (r *Runtime) DeleteMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "mcp.servers.delete", request, commandOptions(options))
}

// TestMCPServer probes an MCP server candidate without persisting it.
func (r *Runtime) TestMCPServer(ctx context.Context, request protocol.MCPServerCandidate, options CallOptions) (*protocol.MCPTestResult, error) {
	return invoke[protocol.MCPServerCandidate, *protocol.MCPTestResult](ctx, r, "mcp.servers.test", request, callOptions(options))
}

// ListMCPTools returns tools advertised by configured MCP servers.
func (r *Runtime) ListMCPTools(ctx context.Context, request protocol.MCPListToolsRequest, options CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	return invoke[protocol.MCPListToolsRequest, *protocol.Page[protocol.MCPTool]](ctx, r, "mcp.tools.list", request, callOptions(options))
}

// ReconnectMCPServer reconnects one enabled MCP server.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, request protocol.MCPServerRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "mcp.servers.reconnect", request, commandOptions(options))
}

// CreateMCPAuthorizationAttempt starts browser authorization for an MCP server.
func (r *Runtime) CreateMCPAuthorizationAttempt(ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return invoke[protocol.CreateMCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, r, "mcp.authorizationAttempts.create", request, commandOptions(options))
}

// GetMCPAuthorizationAttempt returns one MCP authorization attempt.
func (r *Runtime) GetMCPAuthorizationAttempt(ctx context.Context, request protocol.MCPAuthorizationAttemptRequest, options CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	return invoke[protocol.MCPAuthorizationAttemptRequest, *protocol.MCPAuthorizationAttempt](ctx, r, "mcp.authorizationAttempts.get", request, callOptions(options))
}

// ListHooks returns workspace hook discovery and trust state.
func (r *Runtime) ListHooks(ctx context.Context, request protocol.ListHooksRequest, options CallOptions) (*protocol.HooksListResult, error) {
	return invoke[protocol.ListHooksRequest, *protocol.HooksListResult](ctx, r, "hooks.list", request, callOptions(options))
}

// SetHookTrust changes trust for a workspace hook configuration.
func (r *Runtime) SetHookTrust(ctx context.Context, request protocol.SetHookTrustRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "hooks.setTrust", request, commandOptions(options))
}

// ListProviders returns the provider registry projection.
func (r *Runtime) ListProviders(ctx context.Context, options CallOptions) (*protocol.Page[protocol.Provider], error) {
	return invoke[struct{}, *protocol.Page[protocol.Provider]](ctx, r, "providers.list", struct{}{}, callOptions(options))
}

// UpdateProvider updates credentials or endpoint settings for one provider.
func (r *Runtime) UpdateProvider(ctx context.Context, request protocol.UpdateProviderRequest, options CommandOptions) (*protocol.Provider, error) {
	return invoke[protocol.UpdateProviderRequest, *protocol.Provider](ctx, r, "providers.update", request, commandOptions(options))
}

// TestProvider probes one provider without mutating its configuration.
func (r *Runtime) TestProvider(ctx context.Context, request protocol.TestProviderRequest, options CallOptions) (*protocol.ProviderTestResult, error) {
	return invoke[protocol.TestProviderRequest, *protocol.ProviderTestResult](ctx, r, "providers.test", request, callOptions(options))
}

// ListModels returns models available through configured providers.
func (r *Runtime) ListModels(ctx context.Context, request protocol.ListModelsRequest, options CallOptions) (*protocol.Page[protocol.Model], error) {
	return invoke[protocol.ListModelsRequest, *protocol.Page[protocol.Model]](ctx, r, "models.list", request, callOptions(options))
}

// GetUtilityRole returns the model used for maintenance work.
func (r *Runtime) GetUtilityRole(ctx context.Context, options CallOptions) (*protocol.UtilityRole, error) {
	return invoke[struct{}, *protocol.UtilityRole](ctx, r, "models.getUtilityRole", struct{}{}, callOptions(options))
}

// SetUtilityRole changes the model used for maintenance work.
func (r *Runtime) SetUtilityRole(ctx context.Context, request protocol.UtilityRole, options CommandOptions) (*protocol.UtilityRole, error) {
	return invoke[protocol.UtilityRole, *protocol.UtilityRole](ctx, r, "models.setUtilityRole", request, commandOptions(options))
}

// GetEmbeddingRole returns the model used for embeddings.
func (r *Runtime) GetEmbeddingRole(ctx context.Context, options CallOptions) (*protocol.EmbeddingRole, error) {
	return invoke[struct{}, *protocol.EmbeddingRole](ctx, r, "models.getEmbeddingRole", struct{}{}, callOptions(options))
}

// SetEmbeddingRole changes the model used for embeddings.
func (r *Runtime) SetEmbeddingRole(ctx context.Context, request protocol.EmbeddingRole, options CommandOptions) (*protocol.EmbeddingRole, error) {
	return invoke[protocol.EmbeddingRole, *protocol.EmbeddingRole](ctx, r, "models.setEmbeddingRole", request, commandOptions(options))
}

// ListTools returns the Runtime's model-facing tool descriptors.
func (r *Runtime) ListTools(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	return invoke[struct{}, *protocol.Page[protocol.ToolSpec]](ctx, r, "tools.list", struct{}{}, callOptions(options))
}

// InvokeTool invokes one Runtime tool outside an Agent Run.
func (r *Runtime) InvokeTool(ctx context.Context, request protocol.InvokeToolRequest, options CommandOptions) (any, error) {
	return invoke[protocol.InvokeToolRequest, any](ctx, r, "tools.invoke", request, commandOptions(options))
}

// SearchCodebase searches the semantic codebase index.
func (r *Runtime) SearchCodebase(ctx context.Context, request protocol.CodebaseSearchRequest, options CallOptions) (*protocol.CodebaseSearchResult, error) {
	return invoke[protocol.CodebaseSearchRequest, *protocol.CodebaseSearchResult](ctx, r, "codebase.search", request, callOptions(options))
}

// GetCodebaseStatus returns index status for a workspace.
func (r *Runtime) GetCodebaseStatus(ctx context.Context, request protocol.CodebaseStatusRequest, options CallOptions) (*protocol.CodebaseStatus, error) {
	return invoke[protocol.CodebaseStatusRequest, *protocol.CodebaseStatus](ctx, r, "codebase.status", request, callOptions(options))
}

// ReindexCodebase requests a fresh workspace index.
func (r *Runtime) ReindexCodebase(ctx context.Context, request protocol.CodebaseReindexRequest, options CommandOptions) (*protocol.CodebaseReindexResponse, error) {
	return invoke[protocol.CodebaseReindexRequest, *protocol.CodebaseReindexResponse](ctx, r, "codebase.reindex", request, commandOptions(options))
}
