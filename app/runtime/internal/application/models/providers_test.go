package models

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

type testProviderRegistry struct {
	entries   map[string]provider.Provider
	updates   []provider.Patch
	getErr    error
	updateErr error
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

func (r *testProviderRegistry) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	if r.updateErr != nil {
		return provider.Provider{}, r.updateErr
	}
	r.updates = append(r.updates, patch)
	if r.entries == nil {
		r.entries = map[string]provider.Provider{}
	}
	entry := r.entries[id]
	entry.ID = id
	entry = entry.Apply(patch)
	r.entries[id] = entry
	return entry, nil
}

type testCatalog struct {
	metadata []ProviderMetadata
	models   map[string][]Model
}

func (c testCatalog) Supported() []ProviderMetadata { return slices.Clone(c.metadata) }

func (c testCatalog) Metadata(id string) (ProviderMetadata, bool) {
	for _, metadata := range c.metadata {
		if metadata.ID == id {
			return metadata, true
		}
	}
	return ProviderMetadata{}, false
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
		metadata: []ProviderMetadata{{ID: "ollama", ProbeModels: true}},
		models:   map[string][]Model{"ollama": {{ID: "known", Provider: "ollama", Details: &Details{DisplayName: "Known"}}}},
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
		metadata: []ProviderMetadata{{ID: "ollama", ProbeModels: true}},
		models:   map[string][]Model{"ollama": {{ID: "fallback", Provider: "ollama", Details: &Details{}}}},
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
			metadata: []ProviderMetadata{{ID: "anthropic"}},
			models:   map[string][]Model{"anthropic": {{ID: "cataloged", Provider: "anthropic", Details: &Details{}}}},
		},
		Lister: lister,
	})

	got := c.ListModels(t.Context(), "anthropic")
	if len(got) != 1 || got[0].ID != "cataloged" || lister.gotEntry.ID != "" {
		t.Fatalf("models=%+v lister=%+v", got, lister.gotEntry)
	}
}

func TestUpdateProviderOwnsSupportAndBaseURLPolicy(t *testing.T) {
	registry := &testProviderRegistry{}
	c := New(Config{
		Providers: registry,
		Catalog:   testCatalog{metadata: []ProviderMetadata{{ID: "compat", RequiresBaseURL: true}}},
	})

	apiKey := "sk-secret"
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: &apiKey}); !errors.Is(err, ErrProviderBaseURLRequired) {
		t.Fatalf("missing base URL error = %v", err)
	}
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "unknown", APIKey: &apiKey}); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unknown provider error = %v", err)
	}
	baseURL := "https://example.test"
	configured, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: &apiKey, BaseURL: &baseURL})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(registry.updates) != 1 || configured.APIKeyMasked == "" || configured.APIKeyMasked == "sk-secret" {
		t.Fatalf("updated=%+v patches=%+v", configured, registry.updates)
	}

	replacement := "sk-replaced"
	configured, err = c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", APIKey: &replacement})
	if err != nil {
		t.Fatalf("update key while preserving endpoint: %v", err)
	}
	if configured.BaseURL != baseURL {
		t.Fatalf("base URL = %q, want preserved %q", configured.BaseURL, baseURL)
	}

	emptyBaseURL := ""
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat", BaseURL: &emptyBaseURL}); !errors.Is(err, ErrProviderBaseURLRequired) {
		t.Fatalf("clear required base URL error = %v", err)
	}
	if _, err := c.UpdateProvider(t.Context(), UpdateProviderCommand{ID: "compat"}); !errors.Is(err, ErrProviderUpdateRequired) {
		t.Fatalf("empty update error = %v", err)
	}
}

func TestTestProviderRequiresAConfiguredSupportedProvider(t *testing.T) {
	prober := &fakeProber{}
	c := New(Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "sk-secret"},
		}},
		Catalog: testCatalog{metadata: []ProviderMetadata{{ID: "anthropic"}}},
		Prober:  prober,
	})

	if _, err := c.TestProvider(t.Context(), "missing"); !errors.Is(err, ErrProviderUnsupported) {
		t.Fatalf("unsupported error = %v", err)
	}
	if outcome, err := c.TestProvider(t.Context(), "anthropic"); err != nil || outcome != ProviderTestSucceeded {
		t.Fatalf("test provider = %q, %v", outcome, err)
	}
	if prober.got.ID != "anthropic" {
		t.Fatalf("probed = %+v", prober.got)
	}
}
