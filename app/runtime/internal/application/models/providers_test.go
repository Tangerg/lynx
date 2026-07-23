package models

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

type testProviderRegistry struct {
	entries      map[string]provider.Provider
	configured   []provider.Provider
	getErr       error
	configureErr error
	dropReadBack bool
}

func (r *testProviderRegistry) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry)
	}
	return out, nil
}

func (r *testProviderRegistry) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	if r.getErr != nil {
		return provider.Provider{}, false, r.getErr
	}
	entry, ok := r.entries[id]
	return entry, ok, nil
}

func (r *testProviderRegistry) Configure(_ context.Context, entry provider.Provider) error {
	if r.configureErr != nil {
		return r.configureErr
	}
	r.configured = append(r.configured, entry)
	if r.dropReadBack {
		return nil
	}
	if r.entries == nil {
		r.entries = map[string]provider.Provider{}
	}
	r.entries[entry.ID] = entry
	return nil
}

type testCatalog struct {
	metadata []provider.Metadata
	models   map[string][]Model
}

func (c testCatalog) Supported() []provider.Metadata { return slices.Clone(c.metadata) }

func (c testCatalog) Metadata(id string) (provider.Metadata, bool) {
	for _, metadata := range c.metadata {
		if metadata.ID == id {
			return metadata, true
		}
	}
	return provider.Metadata{}, false
}

func (c testCatalog) Models(providerID string) []Model {
	return slices.Clone(c.models[providerID])
}

func (c testCatalog) LookupModel(providerID, modelID string) (Model, bool) {
	for _, model := range c.models[providerID] {
		if model.ID == modelID {
			return model, true
		}
	}
	return Model{}, false
}

type fakeLister struct {
	gotEntry provider.Provider
	ids      []string
	err      error
}

func (l *fakeLister) ListModels(_ context.Context, entry provider.Provider) ([]string, error) {
	l.gotEntry = entry
	return l.ids, l.err
}

type fakeProber struct {
	got provider.Provider
	err error
}

func (p *fakeProber) Probe(_ context.Context, entry provider.Provider) error {
	p.got = entry
	return p.err
}

func TestListModelsPrefersRemoteModelsAndEnrichesKnownEntries(t *testing.T) {
	registry := &testProviderRegistry{entries: map[string]provider.Provider{
		"ollama": {ID: "ollama", BaseURL: "http://host:1234/v1", APIKey: "k"},
	}}
	catalog := testCatalog{
		metadata: []provider.Metadata{{ID: "ollama", ProbeModels: true}},
		models:   map[string][]Model{"ollama": {{ID: "known", Provider: "ollama", Details: &ModelDetails{DisplayName: "Known"}}}},
	}
	lister := &fakeLister{ids: []string{"known", "local"}}
	c := New(Config{Providers: registry, Catalog: catalog, Lister: lister})

	got := c.ListModels(t.Context(), "ollama")
	if len(got) != 2 || got[0].Details == nil || got[0].Details.DisplayName != "Known" || got[1].ID != "local" || got[1].Details != nil {
		t.Fatalf("models = %+v", got)
	}
	if lister.gotEntry.BaseURL != "http://host:1234/v1" || lister.gotEntry.APIKey != "k" {
		t.Fatalf("lister entry = %+v, want configured endpoint + key", lister.gotEntry)
	}
}

func TestListModelsFallsBackToStaticCatalogWhenProbeCannotAnswer(t *testing.T) {
	catalog := testCatalog{
		metadata: []provider.Metadata{{ID: "ollama", ProbeModels: true}},
		models:   map[string][]Model{"ollama": {{ID: "fallback", Provider: "ollama", Details: &ModelDetails{}}}},
	}
	c := New(Config{
		Providers: &testProviderRegistry{},
		Catalog:   catalog,
		Lister:    &fakeLister{err: errors.New("offline")},
	})

	got := c.ListModels(t.Context(), "ollama")
	if len(got) != 1 || got[0].ID != "fallback" {
		t.Fatalf("models = %+v, want static fallback", got)
	}
}

func TestListModelsSkipsRemoteProbeForStaticProvider(t *testing.T) {
	lister := &fakeLister{ids: []string{"must-not-appear"}}
	c := New(Config{
		Catalog: testCatalog{
			metadata: []provider.Metadata{{ID: "anthropic"}},
			models:   map[string][]Model{"anthropic": {{ID: "cataloged", Provider: "anthropic", Details: &ModelDetails{}}}},
		},
		Lister: lister,
	})

	got := c.ListModels(t.Context(), "anthropic")
	if len(got) != 1 || got[0].ID != "cataloged" || lister.gotEntry.ID != "" {
		t.Fatalf("models=%+v lister=%+v", got, lister.gotEntry)
	}
}

func TestConfigureProviderOwnsSupportAndBaseURLPolicy(t *testing.T) {
	registry := &testProviderRegistry{}
	c := New(Config{
		Providers: registry,
		Catalog:   testCatalog{metadata: []provider.Metadata{{ID: "compat", RequiresBaseURL: true}}},
	})

	if _, err := c.ConfigureProvider(t.Context(), ConfigureProviderCommand{ID: "compat", APIKey: "sk-secret"}); !errors.Is(err, ErrProviderBaseURLRequired) {
		t.Fatalf("missing base URL error = %v", err)
	}
	if _, err := c.ConfigureProvider(t.Context(), ConfigureProviderCommand{ID: "unknown"}); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unknown provider error = %v", err)
	}
	configured, err := c.ConfigureProvider(t.Context(), ConfigureProviderCommand{ID: "compat", APIKey: "sk-secret", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(registry.configured) != 1 || configured.APIKeyMasked == "" || configured.APIKeyMasked == "sk-secret" {
		t.Fatalf("configured=%+v stored=%+v", configured, registry.configured)
	}
}

func TestTestProviderRequiresAConfiguredSupportedProvider(t *testing.T) {
	prober := &fakeProber{}
	c := New(Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "sk-secret"},
		}},
		Catalog: testCatalog{metadata: []provider.Metadata{{ID: "anthropic"}}},
		Prober:  prober,
	})

	if err := c.TestProvider(t.Context(), "missing"); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unsupported error = %v", err)
	}
	if err := c.TestProvider(t.Context(), "anthropic"); err != nil {
		t.Fatalf("test provider: %v", err)
	}
	if prober.got.ID != "anthropic" {
		t.Fatalf("probed = %+v", prober.got)
	}
}
