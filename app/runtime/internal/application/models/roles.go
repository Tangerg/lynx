package models

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// Role is the application read model for a configured model role. It avoids
// exposing the domain value object at the Delivery boundary while keeping the
// paired provider/model result atomic.
type Role struct {
	Provider string
	Model    string
}

// UtilityRole returns the live utility-model role; both empty when unset
// (maintenance runs on the main turn model). Backs models.getUtilityRole.
func (c *Coordinator) UtilityRole() Role {
	role := c.utilityRoleState.Role()
	return Role{Provider: role.ProviderID(), Model: role.Model()}
}

// SetUtilityRole repoints the maintenance services at (provider, model), persists
// it, and swaps the live cell so the change takes effect at the next turn
// boundary. An empty model clears the role back to the main turn model. A
// non-empty model is validated before persistence — an unsupported or
// unconfigured provider fails here rather than silently degrading at the next
// compaction. Backs models.setUtilityRole.
func (c *Coordinator) SetUtilityRole(ctx context.Context, provider, model string) (Role, error) {
	c.utilityMu.Lock()
	defer c.utilityMu.Unlock()
	role, err := modelrole.New(provider, model)
	if err != nil {
		return Role{}, err
	}
	if role.Configured() {
		if _, _, err := c.configuredProvider(ctx, role.ProviderID()); err != nil {
			return Role{}, err
		}
		if c.utilityValidator == nil {
			return Role{}, errors.New("models: utility model validation is unavailable")
		}
		if err := c.utilityValidator.ValidateChatModel(ctx, role.ProviderID(), role.Model()); err != nil {
			return Role{}, fmt.Errorf("models: utility model %q on %q: %w", role.Model(), role.ProviderID(), err)
		}
	}
	if c.utilityStore != nil {
		if err := c.utilityStore.SaveUtilityRole(ctx, role.ProviderID(), role.Model()); err != nil {
			return Role{}, err
		}
	}
	c.utilityRoleState.Store(role)
	return Role{Provider: role.ProviderID(), Model: role.Model()}, nil
}

// EmbeddingRole returns the live embedding role; both empty when unset. Backs
// models.getEmbeddingRole.
func (c *Coordinator) EmbeddingRole() Role {
	role := c.embeddingRoleState.Role()
	return Role{Provider: role.ProviderID(), Model: role.Model()}
}

// SetEmbeddingRole repoints the @codebase index at (provider, model), persists
// it, and swaps the live cell. An empty model clears the role (turns the index
// off). A non-empty model is validated by building its embedding client, so an
// unsupported, unconfigured, or unbuildable role fails here rather than at the
// next search. Backs models.setEmbeddingRole.
func (c *Coordinator) SetEmbeddingRole(ctx context.Context, providerID, model string) (Role, error) {
	c.embeddingMu.Lock()
	defer c.embeddingMu.Unlock()
	role, err := modelrole.New(providerID, model)
	if err != nil {
		return Role{}, err
	}
	if role.Configured() {
		meta, _, err := c.configuredProvider(ctx, role.ProviderID())
		if err != nil {
			return Role{}, err
		}
		if !meta.EmbeddingCapable {
			return Role{}, fmt.Errorf("%w: provider %q", ErrEmbeddingUnsupported, role.ProviderID())
		}
		if c.embeddingResolver == nil {
			return Role{}, errors.New("models: embedding model validation is unavailable")
		}
		if _, err := c.embeddingResolver.Resolve(ctx, role.ProviderID(), role.Model()); err != nil {
			return Role{}, fmt.Errorf("models: build embedding model %q on %q: %w", role.Model(), role.ProviderID(), err)
		}
	}
	if c.embeddingStore != nil {
		if err := c.embeddingStore.SaveEmbeddingRole(ctx, role.ProviderID(), role.Model()); err != nil {
			return Role{}, fmt.Errorf("models: persist embedding role: %w", err)
		}
	}
	c.embeddingRoleState.Store(role)
	return Role{Provider: role.ProviderID(), Model: role.Model()}, nil
}
