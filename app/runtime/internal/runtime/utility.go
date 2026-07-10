package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/core/model/chat"
)

// UtilityRoleStore persists the global utility-model role across restarts.
// Defined here (consumer side); the composition root injects the sqlite-backed
// implementation. A nil store disables persistence — the role stays unset.
type UtilityRoleStore interface {
	utilityRoleLoader
	utilityRoleSaver
}

type utilityRoleLoader interface {
	LoadUtilityRole(ctx context.Context) (provider, model string, err error)
}

type utilityRoleSaver interface {
	SaveUtilityRole(ctx context.Context, provider, model string) error
}

type chatClientResolver interface {
	ResolveClient(ctx context.Context, providerID, model string) (*chat.Client, error)
}

type utilityEnvironment struct {
	cell    *atomic.Pointer[modelrole.Role]
	resolve func(context.Context) *chat.Client
}

func buildUtilityEnvironment(ctx context.Context, mainClient *chat.Client, loader utilityRoleLoader, resolver chatClientResolver) (utilityEnvironment, error) {
	var role modelrole.Role
	if loader != nil {
		p, m, err := loader.LoadUtilityRole(ctx)
		if err != nil {
			return utilityEnvironment{}, fmt.Errorf("runtime: load utility role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return utilityEnvironment{}, fmt.Errorf("runtime: load utility role: %w", err)
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

// UtilityRole returns the live utility-model role; both empty when unset
// (maintenance runs on the main turn model). Backs models.getUtilityRole.
func (r *Runtime) UtilityRole() (provider, model string) {
	role := r.utility.Load()
	if role == nil {
		return "", ""
	}
	return role.ProviderID(), role.Model()
}

// SetUtilityRole repoints the maintenance services at (provider, model),
// persists it, and swaps the live cell so the change takes effect at the next
// turn boundary. An empty model clears the role back to the main turn model. A
// non-empty model is validated by resolving its client first — an unconfigured
// provider or unknown model fails here (surfaced to the caller) rather than
// silently degrading at the next compaction. Backs models.setUtilityRole.
func (r *Runtime) SetUtilityRole(ctx context.Context, provider, model string) error {
	role, err := modelrole.New(provider, model)
	if err != nil {
		return err
	}
	if role.Configured() {
		if _, err := r.utilityClients.ResolveClient(ctx, role.ProviderID(), role.Model()); err != nil {
			return fmt.Errorf("runtime: utility model %q on %q: %w", role.Model(), role.ProviderID(), err)
		}
	}
	if r.utilStore != nil {
		if err := r.utilStore.SaveUtilityRole(ctx, role.ProviderID(), role.Model()); err != nil {
			return err
		}
	}
	r.utility.Store(&role)
	return nil
}
