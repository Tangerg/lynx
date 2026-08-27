package bootstrap

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/application/models"
	"github.com/Tangerg/scope/app/runtime/internal/config"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/provider"
)

// SeedConfiguredProvider ensures the config-file provider is present in the
// registry with its key, so the default provider is enabled on first run. A
// provider enabled by a stored key is left untouched — runtime edits win over
// the config file. An environment key is never copied into storage; only a
// missing configured endpoint is seeded beside it.
func SeedConfiguredProvider(ctx context.Context, registry models.ProviderRegistry, cfg config.Settings) error {
	id := cfg.Provider
	existing, ok, err := registry.Get(ctx, id)
	if err != nil {
		return err
	}
	if ok && existing.Enabled() {
		if existing.KeySource != provider.KeyEnv || existing.BaseURL != "" || cfg.BaseURL == "" {
			return nil
		}
		_, err = registry.Update(ctx, id, provider.Patch{BaseURL: &cfg.BaseURL})
		return err
	}
	_, err = registry.Update(ctx, id, provider.Patch{
		APIKey:  &cfg.APIKey,
		BaseURL: &cfg.BaseURL,
	})
	return err
}

// SeedUtilityRole writes the config-file utility model into the store on first
// run (when no row exists yet), pinned to the default provider. A role already
// persisted via models.setUtilityRole is left untouched — runtime edits win
// over the config file. An empty / identical-to-main UtilityModel seeds
// nothing (maintenance then runs on the main model).
func SeedUtilityRole(ctx context.Context, store UtilityRoleStore, cfg config.Settings) error {
	role, err := store.LoadUtilityRole(ctx)
	if err != nil {
		return err
	}
	if role.Configured() {
		return nil
	}
	if cfg.UtilityModel == "" || cfg.UtilityModel == cfg.Model {
		return nil
	}
	role, err = modelref.New(cfg.Provider, cfg.UtilityModel)
	if err != nil {
		return err
	}
	return store.SaveUtilityRole(ctx, role)
}
