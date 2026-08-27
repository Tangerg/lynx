package bootstrap

import (
	"context"
	"fmt"

	mcpapp "github.com/Tangerg/scope/app/runtime/internal/application/mcp"
	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

// mcpServerList is the boot-time snapshot view of the MCP registry: building the
// live policy + dial descriptors needs only one List, not configure/remove.
type mcpServerList interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
}

// mcpEnvironment is the boot-time MCP material: the application-owned live
// policy state and the enabled durable server definitions to connect.
type mcpEnvironment struct {
	policy  *mcpapp.ToolPolicyState
	servers []mcpserver.Server
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
	servers, err := registry.List(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("bootstrap: load mcp registry: %w", err)
	}
	policy := mcpserver.NewToolPolicy(servers)
	return mcpEnvironment{
		policy:  mcpapp.NewToolPolicyState(policy),
		servers: enabledMCPServers(servers),
	}, nil
}

func enabledMCPServers(servers []mcpserver.Server) []mcpserver.Server {
	var enabled []mcpserver.Server
	for _, server := range servers {
		if server.Enabled {
			enabled = append(enabled, server)
		}
	}
	return enabled
}
