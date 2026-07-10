package provider

import (
	"cmp"
	"context"
	"maps"
	"slices"
)

// envKeyRegistry decorates a registry so a provider with no stored key falls
// back to its environment-variable key. Precedence is stored > env: a key set
// via providers.configure always wins over the environment. The decorator is
// the single authority on [Provider.KeySource] — it's the only layer that knows
// whether the effective key is stored or env-sourced.
type envKeyRegistry struct {
	inner   Registry
	envKeys map[string]string // provider id -> env key value (non-empty)
}

// WithEnvKeys wraps a registry with the stored>env credential fallback: a
// provider absent or keyless in inner becomes enabled when its id has an entry
// in envKeys, with [Provider.KeySource] set to [KeyEnv]. envKeys (from
// llm.EnvKeys, read once at startup) is copied into an immutable snapshot. An
// empty map makes this a transparent pass-through, so the decorator is free to
// apply always.
func WithEnvKeys(inner Registry, envKeys map[string]string) Registry {
	if len(envKeys) == 0 {
		return inner
	}
	return &envKeyRegistry{inner: inner, envKeys: maps.Clone(envKeys)}
}

// resolve stamps KeySource and overlays the env key when there's no stored one.
// found mirrors the inner Get's ok — but an env-only provider (no stored row)
// still resolves as found, since an env key makes it usable.
func (s *envKeyRegistry) resolve(p Provider, found bool, id string) (Provider, bool) {
	if found && p.APIKey != "" {
		p.KeySource = KeyStored
		return p, true
	}
	if env := s.envKeys[id]; env != "" {
		// Overlay onto the stored row (keeps any configured base URL) or
		// synthesize a fresh entry for an env-only provider.
		p.ID = id
		p.APIKey = env
		p.KeySource = KeyEnv
		return p, true
	}
	if found {
		p.KeySource = KeyNone
		return p, true
	}
	return Provider{}, false
}

func (s *envKeyRegistry) Get(ctx context.Context, id string) (Provider, bool, error) {
	p, ok, err := s.inner.Get(ctx, id)
	if err != nil {
		return Provider{}, false, err
	}
	rp, rok := s.resolve(p, ok, id)
	return rp, rok, nil
}

func (s *envKeyRegistry) List(ctx context.Context) ([]Provider, error) {
	stored, err := s.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(stored)+len(s.envKeys))
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
		out = append(out, Provider{ID: id, APIKey: env, KeySource: KeyEnv})
	}
	slices.SortFunc(out, func(a, b Provider) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// Configure passes through: env keys are read-only, never persisted. A stored
// key written here takes precedence over the environment on subsequent reads.
func (s *envKeyRegistry) Configure(ctx context.Context, p Provider) error {
	return s.inner.Configure(ctx, p)
}
