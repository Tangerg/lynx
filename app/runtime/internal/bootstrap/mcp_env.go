package bootstrap

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// mcpServerList is the boot-time snapshot view of the MCP registry: building the
// live policy + dial descriptors needs only one List, not configure/remove.
type mcpServerList interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
}

// mcpEnvironment is the boot-time MCP wiring: the live tool policy cell (shared
// with the capabilities coordinator's MCP refresh path), the gating predicates
// the toolset + approval read, and the enabled-server dial descriptors the
// engine's tools are built from.
type mcpEnvironment struct {
	policy           *atomic.Pointer[mcpserver.ToolPolicy]
	toolDisabled     func(mcpserver.ToolRef) bool
	toolAutoApproved func(mcpserver.ToolRef) bool
	configs          []mcpserver.LiveConfig
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
	servers, err := registry.List(ctx)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("bootstrap: load mcp registry: %w", err)
	}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(servers)
	policyCell.Store(&policy)
	return mcpEnvironment{
		policy: policyCell,
		toolDisabled: func(ref mcpserver.ToolRef) bool {
			return policyCell.Load().Disabled(ref)
		},
		toolAutoApproved: func(ref mcpserver.ToolRef) bool {
			return policyCell.Load().AutoApproved(ref)
		},
		configs: mcpserver.ConfigsForEnabledServers(servers),
	}, nil
}
