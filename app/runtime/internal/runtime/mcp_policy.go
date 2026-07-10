package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

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
