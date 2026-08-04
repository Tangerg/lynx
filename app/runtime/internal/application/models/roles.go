package models

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// UtilityRole returns the live utility-model role; both empty when unset
// (maintenance runs on the main Run model). Backs models.getUtilityRole.
func (c *Coordinator) UtilityRole() modelref.Selection { return c.utilityRoleState.Role() }

// SetUtilityRole repoints the maintenance services at (provider, model), persists
// it, and swaps the live cell so the change takes effect at the next Run
// boundary. An empty model clears the role back to the main Run model. A
// non-empty model is validated before persistence — an unsupported or
// unconfigured provider fails here rather than silently degrading at the next
// compaction. Backs models.setUtilityRole.
func (c *Coordinator) SetUtilityRole(ctx context.Context, provider, model string) (modelref.Selection, error) {
	c.utilityMu.Lock()
	defer c.utilityMu.Unlock()
	role, err := modelref.New(provider, model)
	if err != nil {
		return modelref.Selection{}, err
	}
	if role.Configured() {
		if _, _, err := c.configuredProvider(ctx, role.Provider()); err != nil {
			return modelref.Selection{}, err
		}
		if c.utilityValidator == nil {
			return modelref.Selection{}, errors.New("models: utility model validation is unavailable")
		}
		if err := c.utilityValidator.ValidateChatModel(ctx, role.Provider(), role.Model()); err != nil {
			return modelref.Selection{}, fmt.Errorf("models: utility model %q on %q: %w", role.Model(), role.Provider(), err)
		}
	}
	if c.utilityStore != nil {
		if err := c.utilityStore.SaveUtilityRole(ctx, role); err != nil {
			return modelref.Selection{}, err
		}
	}
	c.utilityRoleState.Store(role)
	return role, nil
}

// EmbeddingRole returns the live embedding role; both empty when unset. Backs
// models.getEmbeddingRole.
func (c *Coordinator) EmbeddingRole() modelref.Selection { return c.embeddingRoleState.Role() }

// SetEmbeddingRole repoints the @codebase index at (provider, model), persists
// it, and swaps the live cell. An empty model clears the role (turns the index
// off). A non-empty model is validated by building its embedding client, so an
// unsupported, unconfigured, or unbuildable role fails here rather than at the
// next search. Backs models.setEmbeddingRole.
func (c *Coordinator) SetEmbeddingRole(ctx context.Context, providerID, model string) (modelref.Selection, error) {
	c.embeddingMu.Lock()
	defer c.embeddingMu.Unlock()
	role, err := modelref.New(providerID, model)
	if err != nil {
		return modelref.Selection{}, err
	}
	if role.Configured() {
		meta, _, err := c.configuredProvider(ctx, role.Provider())
		if err != nil {
			return modelref.Selection{}, err
		}
		if !meta.EmbeddingCapable {
			return modelref.Selection{}, fmt.Errorf("%w: provider %q", ErrEmbeddingUnsupported, role.Provider())
		}
		if c.embeddingValidator == nil {
			return modelref.Selection{}, errors.New("models: embedding model validation is unavailable")
		}
		if err := c.embeddingValidator.ValidateEmbeddingModel(ctx, role.Provider(), role.Model()); err != nil {
			return modelref.Selection{}, fmt.Errorf("models: build embedding model %q on %q: %w", role.Model(), role.Provider(), err)
		}
	}
	if c.embeddingStore != nil {
		if err := c.embeddingStore.SaveEmbeddingRole(ctx, role); err != nil {
			return modelref.Selection{}, fmt.Errorf("models: persist embedding role: %w", err)
		}
	}
	c.embeddingRoleState.Store(role)
	return role, nil
}
