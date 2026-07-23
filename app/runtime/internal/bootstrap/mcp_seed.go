package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

type mcpServerSeeder interface {
	Get(ctx context.Context, name string) (mcpserver.Server, bool, error)
	Configure(ctx context.Context, server mcpserver.Server) error
}

// SeedMCPServers writes any env-sourced servers (LYRA_MCP_SERVERS) into the
// registry that aren't already present, mirroring bootstrap.SeedConfiguredProvider: the
// env is a first-run seed, runtime edits (a persisted configure) win and are
// left untouched.
func SeedMCPServers(ctx context.Context, svc mcpServerSeeder, servers []mcpserver.Server) error {
	for _, srv := range servers {
		if _, ok, err := svc.Get(ctx, srv.Name); err != nil {
			return err
		} else if ok {
			continue
		}
		if err := svc.Configure(ctx, srv); err != nil {
			return err
		}
	}
	return nil
}
