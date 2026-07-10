package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

type mcpEnvironment struct {
	policy           *atomic.Pointer[mcpserver.ToolPolicy]
	toolDisabled     func(string) bool
	toolAutoApproved func(string) bool
	configs          []toolport.MCPServerConfig
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
	servers, err := registry.List(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("runtime: load mcp registry: %w", err)
	}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(servers)
	policyCell.Store(&policy)
	return mcpEnvironment{
		policy: policyCell,
		toolDisabled: func(toolName string) bool {
			return policyCell.Load().Disabled(toolName)
		},
		toolAutoApproved: func(toolName string) bool {
			return policyCell.Load().AutoApproved(toolName)
		},
		configs: toolport.ConfigsForEnabledServers(servers),
	}, nil
}

func loadMCPToolPolicy(ctx context.Context, registry mcpServerList) (*mcpserver.ToolPolicy, error) {
	servers, err := registry.List(ctx)
	if err != nil {
		return nil, err
	}
	policy := mcpserver.NewToolPolicy(servers)
	return &policy, nil
}

// refreshMCPToolPolicy atomically publishes the policy derived from the
// just-mutated registry for the next tool resolution and approval decision.
func (r *Runtime) refreshMCPToolPolicy(ctx context.Context) error {
	policy, err := loadMCPToolPolicy(ctx, r.mcpRegistry)
	if err != nil {
		return err
	}
	r.mcpPolicy.Store(policy)
	return nil
}
