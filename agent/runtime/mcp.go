package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/mcp"
)

// MCPToolGroupResolver adapts an [*mcp.Provider] into a
// [core.ToolGroupResolver] extension. It answers tool-group requests
// whose Role matches the configured role, returning a
// [core.LazyToolGroup] backed by the provider's cached tool list — so
// the actual MCP listTools RPC fan-out happens on first access, not at
// platform boot.
//
// Wire one resolver per logical role; aggregate multiple MCP servers
// under a single role by feeding them all into the provider's
// [mcp.ProviderConfig.Sources].
type MCPToolGroupResolver struct {
	name     string
	role     string
	provider *mcp.Provider
	metadata core.ToolGroupMetadata
}

// NewMCPToolGroupResolver returns a resolver answering for role,
// delegating tool discovery to provider. Both arguments are required;
// nil/empty inputs panic — they're programming errors that should
// surface at boot.
func NewMCPToolGroupResolver(role string, provider *mcp.Provider) *MCPToolGroupResolver {
	if role == "" {
		panic("runtime.NewMCPToolGroupResolver: role must not be empty")
	}
	if provider == nil {
		panic("runtime.NewMCPToolGroupResolver: provider must not be nil")
	}
	return &MCPToolGroupResolver{
		name:     "mcp-tool-resolver:" + role,
		role:     role,
		provider: provider,
		metadata: core.SimpleToolGroupMetadata{RoleText: role},
	}
}

func (r *MCPToolGroupResolver) Name() string { return r.name }

// Resolve returns a lazy [core.ToolGroup] for matching roles, or
// (nil, nil) — the runtime then falls through to the next registered
// resolver. The lazy group's first Tools() call drives
// [mcp.Provider.Tools]; subsequent calls hit the provider's cache.
func (r *MCPToolGroupResolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, error) {
	if req.Role != r.role {
		return nil, nil
	}
	return core.NewLazyToolGroup(r.metadata, r.provider.Tools), nil
}
