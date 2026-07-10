package startup

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// SeedConfiguredProvider ensures the config-file provider is present in the
// registry with its key, so the default provider is enabled on first run. A
// provider already enabled in the registry (a persisted providers.configure)
// is left untouched — runtime edits win over the config file.
func SeedConfiguredProvider(ctx context.Context, svc providersvc.Registry, cfg config.Config) error {
	id := cfg.Provider
	if existing, ok, err := svc.Get(ctx, id); err != nil {
		return err
	} else if ok && existing.Enabled() {
		return nil
	}
	return svc.Configure(ctx, providersvc.Provider{
		ID:      id,
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
	})
}

// SeedUtilityRole writes the config-file utility model into the store on first
// run (when no row exists yet), pinned to the default provider. A role already
// persisted via models.setUtilityRole is left untouched — runtime edits win
// over the config file. An empty / identical-to-main UtilityModel seeds
// nothing (maintenance then runs on the main model).
func SeedUtilityRole(ctx context.Context, store lyraruntime.UtilityRoleStore, cfg config.Config) error {
	if _, model, err := store.LoadUtilityRole(ctx); err != nil {
		return err
	} else if model != "" {
		return nil
	}
	if cfg.UtilityModel == "" || cfg.UtilityModel == cfg.Model {
		return nil
	}
	role, err := modelrole.New(cfg.Provider, cfg.UtilityModel)
	if err != nil {
		return err
	}
	return store.SaveUtilityRole(ctx, role.ProviderID(), role.Model())
}
