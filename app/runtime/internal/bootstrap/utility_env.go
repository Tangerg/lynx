package bootstrap

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/modelclient"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// utilityRoleLoader is the boot-time load view of the utility-role store
// (persistence save belongs to the runtime facade's SetUtilityRole).
type utilityRoleLoader interface {
	LoadUtilityRole(ctx context.Context) (provider, model string, err error)
}

// utilityEnvironment is the boot-time utility-model wiring: the live role cell
// (repointed by the runtime facade's SetUtilityRole) and a resolve closure that
// yields the utility client, falling back to the main turn client when the role
// is unset or unresolvable.
type utilityEnvironment struct {
	cell    *atomic.Pointer[modelrole.Role]
	resolve func(context.Context) *chat.Client
}

func buildUtilityEnvironment(ctx context.Context, mainClient *chat.Client, loader utilityRoleLoader, resolver *modelclient.ClientResolver) (utilityEnvironment, error) {
	var role modelrole.Role
	if loader != nil {
		p, m, err := loader.LoadUtilityRole(ctx)
		if err != nil {
			return utilityEnvironment{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return utilityEnvironment{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
	}
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(&role)
	resolve := func(ctx context.Context) *chat.Client {
		role := cell.Load()
		if role == nil || !role.Configured() || resolver == nil {
			return mainClient
		}
		c, err := resolver.ResolveClient(ctx, role.ProviderID(), role.Model())
		if err != nil || c == nil {
			return mainClient
		}
		return c
	}
	return utilityEnvironment{cell: cell, resolve: resolve}, nil
}
