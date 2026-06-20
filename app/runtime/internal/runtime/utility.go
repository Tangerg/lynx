package runtime

import (
	"context"
	"fmt"
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
