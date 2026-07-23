package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// mcpServerList is the boot-time snapshot view of the MCP registry: building the
// live policy + dial descriptors needs only one List, not configure/remove.
type mcpServerList interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
}

// mcpEnvironment is the boot-time MCP material: the application-owned live
// policy state and the enabled-server descriptors used to build tools.
type mcpEnvironment struct {
	policy  *integrations.ToolPolicyState
	configs []mcpserver.LiveConfig
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
	servers, err := registry.List(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("bootstrap: load mcp registry: %w", err)
	}
	policy := mcpserver.NewToolPolicy(servers)
	return mcpEnvironment{
		policy:  integrations.NewToolPolicyState(policy),
		configs: mcpserver.ConfigsForEnabledServers(servers),
	}, nil
}
