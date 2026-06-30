package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/core/model/chat"
)

// utilityRole is the (provider, model) the in-house maintenance services
// (compaction / extraction / titling) run on. An empty model means unset —
// those services fall back to the main turn model.
type utilityRole struct {
	provider string
	model    string
}

// UtilityRoleStore persists the global utility-model role across restarts.
// Defined here (consumer side); the composition root injects the sqlite-backed
// implementation. A nil store disables persistence — the role stays unset.
type UtilityRoleStore interface {
	LoadUtilityRole(ctx context.Context) (provider, model string, err error)
	SaveUtilityRole(ctx context.Context, provider, model string) error
}

type utilityEnvironment struct {
	cell    *atomic.Pointer[utilityRole]
	resolve func(context.Context) *chat.Client
}

func buildUtilityEnvironment(ctx context.Context, cfg Config, resolver *clientResolver) (utilityEnvironment, error) {
	var role utilityRole
	if cfg.UtilityRoleStore != nil {
		p, m, err := cfg.UtilityRoleStore.LoadUtilityRole(ctx)
		if err != nil {
			return utilityEnvironment{}, fmt.Errorf("runtime: load utility role: %w", err)
		}
		role = utilityRole{provider: p, model: m}
	}
	cell := &atomic.Pointer[utilityRole]{}
	cell.Store(&role)
	mainClient := cfg.Engine.ChatClient
	resolve := func(ctx context.Context) *chat.Client {
		role := cell.Load()
		if role == nil || role.model == "" {
			return mainClient
		}
		c, err := resolver.ResolveClient(ctx, role.provider, role.model)
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
	return role.provider, role.model
}

// SetUtilityRole repoints the maintenance services at (provider, model),
// persists it, and swaps the live cell so the change takes effect at the next
// turn boundary. An empty model clears the role back to the main turn model. A
// non-empty model is validated by resolving its client first — an unconfigured
// provider or unknown model fails here (surfaced to the caller) rather than
// silently degrading at the next compaction. Backs models.setUtilityRole.
func (r *Runtime) SetUtilityRole(ctx context.Context, provider, model string) error {
	if model == "" {
		provider = "" // a cleared role carries no provider
	} else if _, err := r.resolver.ResolveClient(ctx, provider, model); err != nil {
		return fmt.Errorf("runtime: utility model %q on %q: %w", model, provider, err)
	}
	if r.utilStore != nil {
		if err := r.utilStore.SaveUtilityRole(ctx, provider, model); err != nil {
			return err
		}
	}
	r.utility.Store(&utilityRole{provider: provider, model: model})
	return nil
}
