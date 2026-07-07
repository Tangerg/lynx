package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/mcp"
)

// MCPResolver adapts an [*mcp.Provider] into a
// [core.ToolGroupResolver] extension. It answers tool-group requests
// whose Role matches the configured role, returning a
// [core.LazyToolGroup] backed by the provider's cached tool list — so
// the actual MCP listTools RPC fan-out happens on first access, not at
// platform boot.
//
// Wire one resolver per logical role; aggregate multiple MCP servers
// under a single role by feeding them all into the provider's
// [mcp.ProviderConfig.Sources].
type MCPResolver struct {
	name     string
	role     string
	provider *mcp.Provider
	metadata core.ToolGroupMetadata
}

// NewMCPResolver returns a resolver answering for role,
// delegating tool discovery to provider. Both arguments are required;
// nil/empty inputs return an error — caller decides whether to
// surface or panic.
func NewMCPResolver(role string, provider *mcp.Provider) (*MCPResolver, error) {
	if role == "" {
		return nil, errors.New("runtime.NewMCPResolver: role must not be empty")
	}
	if provider == nil {
		return nil, errors.New("runtime.NewMCPResolver: provider must not be nil")
	}
	return &MCPResolver{
		name:     "mcp-tool-resolver:" + role,
		role:     role,
		provider: provider,
		metadata: core.SimpleToolGroupMetadata{RoleText: role},
	}, nil
}

func (r *MCPResolver) Name() string { return r.name }

// Resolve returns a lazy [core.ToolGroup] for matching roles.
// For unmatched roles it returns ok=false so the runtime falls through to
// the next registered resolver. The lazy group's first Tools() call drives
// [mcp.Provider.Tools]; subsequent calls hit the provider's cache.
func (r *MCPResolver) Resolve(_ context.Context, req core.ToolGroupRequirement) (core.ToolGroup, bool, error) {
	if req.Role != r.role {
		return nil, false, nil
	}
	return core.NewLazyToolGroup(r.metadata, r.provider.Tools), true, nil
}
