package bootstrap

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

type mcpServerSeeder interface {
	Get(ctx context.Context, name string) (mcpserver.Server, bool, error)
	Save(ctx context.Context, server mcpserver.Server) error
}

// SeedMCPServers writes any env-sourced servers (LYRA_MCP_SERVERS) into the
// registry that aren't already present, mirroring bootstrap.SeedConfiguredProvider: the
// env is a first-run seed, runtime edits (a persisted resource) win and are
// left untouched.
func SeedMCPServers(ctx context.Context, registry mcpServerSeeder, servers []mcpserver.Server) error {
	for _, server := range servers {
		if _, ok, err := registry.Get(ctx, server.Name); err != nil {
			return err
		} else if ok {
			continue
		}
		if err := registry.Save(ctx, server); err != nil {
			return err
		}
	}
	return nil
}
