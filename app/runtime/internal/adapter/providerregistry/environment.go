// Package providerregistry decorates the model-provider registry with
// process-environment credential fallback and accurate credential provenance.
package providerregistry

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// envKeyRegistry decorates a registry so a provider with no stored key falls
// back to its environment-variable key. Precedence is stored > env: a key set
// through a registry mutation always wins over the environment. The decorator is
// the single authority on [Provider.KeySource] — it's the only layer that knows
// whether the effective key is stored or env-sourced.
type environmentRegistry struct {
	inner   models.ProviderRegistry
	envKeys map[string]string // provider id -> env key value (non-empty)
}

// WithEnvKeys wraps a registry with the stored>env credential fallback: a
// provider absent or keyless in inner becomes enabled when its id has an entry
// in envKeys, with [Provider.KeySource] set to [KeyEnv]. envKeys (from
// llm.EnvKeys, read once at startup) is copied into an immutable snapshot. An
// empty map still applies the provenance projection so stored credentials are
// reported as [KeyStored] rather than losing their source.
func WithEnvironmentKeys(inner models.ProviderRegistry, envKeys map[string]string) models.ProviderRegistry {
	return &environmentRegistry{inner: inner, envKeys: maps.Clone(envKeys)}
}

// resolve stamps KeySource and overlays the env key when there's no stored one.
// found mirrors the inner Get's ok — but an env-only provider (no stored row)
// still resolves as found, since an env key makes it usable.
func (s *environmentRegistry) resolve(p provider.Provider, found bool, id string) (provider.Provider, bool) {
	if found && p.APIKey != "" {
		p.KeySource = provider.KeyStored
		return p, true
	}
	if env := s.envKeys[id]; env != "" {
		// Overlay onto the stored row (keeps any configured base URL) or
		// synthesize a fresh entry for an env-only provider.
		p.ID = id
		p.APIKey = env
		p.KeySource = provider.KeyEnv
		return p, true
	}
	if found {
		p.KeySource = provider.KeyNone
		return p, true
	}
	return provider.Provider{}, false
}

func (s *environmentRegistry) Get(ctx context.Context, id string) (provider.Provider, bool, error) {
	p, ok, err := s.inner.Get(ctx, id)
	if err != nil {
		return provider.Provider{}, false, err
	}
	rp, rok := s.resolve(p, ok, id)
	return rp, rok, nil
}

func (s *environmentRegistry) List(ctx context.Context) ([]provider.Provider, error) {
	stored, err := s.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Provider, 0, len(stored)+len(s.envKeys))
	seen := make(map[string]struct{}, len(stored))
	for _, p := range stored {
		rp, _ := s.resolve(p, true, p.ID)
		out = append(out, rp)
		seen[p.ID] = struct{}{}
	}
	// Env-only providers (no stored row) still surface as enabled.
	for id, env := range s.envKeys {
		if _, ok := seen[id]; ok {
			continue
		}
		out = append(out, provider.Provider{ID: id, APIKey: env, KeySource: provider.KeyEnv})
	}
	slices.SortFunc(out, func(a, b provider.Provider) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// Update delegates the atomic persisted mutation before resolving the returned
// effective credential. Environment keys remain read-only and are never copied
// into the durable registry.
func (s *environmentRegistry) Update(ctx context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	p, err := s.inner.Update(ctx, id, patch)
	if err != nil {
		return provider.Provider{}, err
	}
	resolved, _ := s.resolve(p, true, id)
	return resolved, nil
}
