package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// providerFake satisfies the models coordinator's three provider-facing
// ports at once: the provider.Registry (List/Get/Update), the static
// ProviderCatalog (Supported/Metadata), and the ProviderProber (Probe).
type providerFake struct {
	entries   map[string]provider.Provider
	supported []models.ProviderMetadata
	updated   []provider.Patch
	probeErr  error
	probed    []provider.Provider
}

func (p *providerFake) Supported() []models.ProviderMetadata {
	if p.supported != nil {
		return p.supported
	}
	return []models.ProviderMetadata{{ID: "anthropic"}}
}

func (p *providerFake) Metadata(id string) (models.ProviderMetadata, bool) {
	for _, meta := range p.Supported() {
		if meta.ID == id {
			return meta, true
		}
	}
	return models.ProviderMetadata{}, false
}
func (*providerFake) Models(string) []models.Model { return nil }
func (*providerFake) LookupModel(string, string) (models.Model, bool) {
	return models.Model{}, false
}

func (p *providerFake) List(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(p.entries))
	for _, entry := range p.entries {
		out = append(out, entry)
	}
	return out, nil
}

func (p *providerFake) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	entry, ok := p.entries[id]
	return entry, ok, nil
}

func (p *providerFake) Update(_ context.Context, id string, patch provider.Patch) (provider.Provider, error) {
	p.updated = append(p.updated, patch)
	if p.entries == nil {
		p.entries = map[string]provider.Provider{}
	}
	entry := p.entries[id]
	entry.ID = id
	entry = entry.Apply(patch)
	p.entries[id] = entry
	return entry, nil
}

func (p *providerFake) Probe(_ context.Context, entry provider.Provider) error {
	p.probed = append(p.probed, entry)
	return p.probeErr
}

func serverWithProviders(rt *providerFake) *Server {
	return serverWithModels(models.Config{Providers: rt, Catalog: rt, Prober: rt})
}

func TestListProvidersMergesSupportedCatalogWithRegistry(t *testing.T) {
	s := serverWithProviders(&providerFake{entries: map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-ant-secret", KeySource: provider.KeyStored},
	}})

	page, err := s.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	var anthropic *protocol.Provider
	for i := range page.Data {
		if page.Data[i].ID == "anthropic" {
			anthropic = &page.Data[i]
			break
		}
	}
	if anthropic == nil {
		t.Fatalf("anthropic missing from supported provider list: %+v", page.Data)
	}
	if anthropic.APIKeyMasked == "" || anthropic.APIKeyMasked == "sk-ant-secret" {
		t.Fatalf("APIKeyMasked = %q, want masked key", anthropic.APIKeyMasked)
	}
	if anthropic.KeySource != protocol.ProviderKeySourceStored {
		t.Fatalf("KeySource = %q, want stored", anthropic.KeySource)
	}
}

func TestUpdateProviderPersistsThenReturnsStoredEntry(t *testing.T) {
	rt := &providerFake{}
	s := serverWithProviders(rt)

	got, err := s.UpdateProvider(context.Background(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey:   setProviderConfig("sk-ant-secret"),
		BaseURL:  setProviderConfig("https://example.test"),
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if len(rt.updated) != 1 {
		t.Fatalf("updated %d provider(s), want 1", len(rt.updated))
	}
	stored := rt.entries["anthropic"]
	if stored.APIKey != "sk-ant-secret" || stored.BaseURL != "https://example.test" {
		t.Fatalf("stored = %+v", stored)
	}
	if got.ID != "anthropic" || got.BaseURL != "https://example.test" || got.APIKeyMasked == "" || got.APIKeyMasked == "sk-ant-secret" {
		t.Fatalf("wire provider = %+v, want masked stored entry", got)
	}
}

func TestUpdateProviderRequiresBaseURLWhenMetadataRequiresIt(t *testing.T) {
	rt := &providerFake{supported: []models.ProviderMetadata{{ID: "openai-compatible", RequiresBaseURL: true}}}
	s := serverWithProviders(rt)

	_, err := s.UpdateProvider(context.Background(), protocol.UpdateProviderRequest{
		Provider: "openai-compatible",
		APIKey:   setProviderConfig("sk-secret"),
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("configure err = %v, want ErrInvalidParams", err)
	}
	if len(rt.updated) != 0 {
		t.Fatalf("updated %d provider(s), want none", len(rt.updated))
	}
}

func TestUpdateProviderPreservesOmittedFieldsAndClearsExplicitly(t *testing.T) {
	rt := &providerFake{entries: map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-ant-secret", BaseURL: "https://old.test"},
	}}
	s := serverWithProviders(rt)

	got, err := s.UpdateProvider(context.Background(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		BaseURL:  clearProviderConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != "" || got.APIKeyMasked == "" {
		t.Fatalf("provider after endpoint clear = %+v, want key preserved", got)
	}

	got, err = s.UpdateProvider(context.Background(), protocol.UpdateProviderRequest{
		Provider: "anthropic",
		APIKey:   clearProviderConfig(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.APIKeyMasked != "" {
		t.Fatalf("provider after key clear = %+v", got)
	}
}

func TestUpdateProviderRejectsAmbiguousConfigChanges(t *testing.T) {
	s := serverWithProviders(&providerFake{})
	empty := ""
	for _, change := range []*protocol.ProviderConfigChange{
		{Type: protocol.ProviderConfigSet},
		{Type: protocol.ProviderConfigSet, Value: &empty},
		{Type: protocol.ProviderConfigClear, Value: &empty},
	} {
		_, err := s.UpdateProvider(t.Context(), protocol.UpdateProviderRequest{
			Provider: "anthropic",
			APIKey:   change,
		})
		if !errors.Is(err, protocol.ErrInvalidParams) {
			t.Fatalf("change %+v error = %v, want invalid params", change, err)
		}
	}
}

func setProviderConfig(value string) *protocol.ProviderConfigChange {
	return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &value}
}

func clearProviderConfig() *protocol.ProviderConfigChange {
	return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigClear}
}

func TestTestProviderUsesConfiguredProvider(t *testing.T) {
	probeErr := errors.New("bad key")
	rt := &providerFake{
		entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "sk-ant-secret"},
		},
		probeErr: probeErr,
	}
	s := serverWithProviders(rt)

	got, err := s.TestProvider(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("test provider: %v", err)
	}
	if got.OK || got.Error == nil || got.Error.Type != "provider_test_failed" || got.Error.Detail != "" {
		t.Fatalf("test result = %+v, want the provider_test_failed symbol and no prose", got)
	}
	if len(rt.probed) != 1 || rt.probed[0].ID != "anthropic" {
		t.Fatalf("probed = %+v, want anthropic", rt.probed)
	}
}
