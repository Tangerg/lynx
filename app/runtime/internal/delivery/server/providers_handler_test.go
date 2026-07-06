package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

type providerRuntime struct {
	stubRuntime
	entries    map[string]provider.Provider
	configured []provider.Provider
	probeErr   error
	probed     []provider.Provider
	dropStored bool
}

func (r *providerRuntime) ListRegisteredProviders(context.Context) ([]provider.Provider, error) {
	out := make([]provider.Provider, 0, len(r.entries))
	for _, entry := range r.entries {
		out = append(out, entry)
	}
	return out, nil
}

func (r *providerRuntime) GetRegisteredProvider(_ context.Context, id string) (provider.Provider, bool, error) {
	entry, ok := r.entries[id]
	return entry, ok, nil
}

func (r *providerRuntime) ConfigureProvider(_ context.Context, entry provider.Provider) error {
	r.configured = append(r.configured, entry)
	if r.dropStored {
		return nil
	}
	if r.entries == nil {
		r.entries = map[string]provider.Provider{}
	}
	r.entries[entry.ID] = entry
	return nil
}

func (r *providerRuntime) ProbeProvider(_ context.Context, entry provider.Provider) error {
	r.probed = append(r.probed, entry)
	return r.probeErr
}

func TestListProvidersMergesSupportedCatalogWithRegistry(t *testing.T) {
	s := newTestServer(&providerRuntime{entries: map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-ant-secret", KeySource: provider.KeyStored},
	}})

	page, err := s.ListProviders(context.Background(), protocol.PageQuery{})
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
	if anthropic.KeySource != string(provider.KeyStored) {
		t.Fatalf("KeySource = %q, want stored", anthropic.KeySource)
	}
}

func TestConfigureProviderPersistsThenReturnsStoredEntry(t *testing.T) {
	rt := &providerRuntime{}
	s := newTestServer(rt)

	got, err := s.ConfigureProvider(context.Background(), protocol.ConfigureProviderRequest{
		Provider: "anthropic",
		APIKey:   "sk-ant-secret",
		BaseURL:  "https://example.test",
	})
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	if len(rt.configured) != 1 {
		t.Fatalf("configured %d provider(s), want 1", len(rt.configured))
	}
	if rt.configured[0].ID != "anthropic" || rt.configured[0].APIKey != "sk-ant-secret" || rt.configured[0].BaseURL != "https://example.test" {
		t.Fatalf("configured = %+v", rt.configured[0])
	}
	if got.ID != "anthropic" || got.BaseURL != "https://example.test" || got.APIKeyMasked == "" || got.APIKeyMasked == "sk-ant-secret" {
		t.Fatalf("wire provider = %+v, want masked stored entry", got)
	}
}

func TestConfigureProviderFailsWhenStoredEntryCannotBeReadBack(t *testing.T) {
	rt := &providerRuntime{dropStored: true}
	s := newTestServer(rt)

	_, err := s.ConfigureProvider(context.Background(), protocol.ConfigureProviderRequest{
		Provider: "anthropic",
		APIKey:   "sk-ant-secret",
	})
	if err == nil {
		t.Fatal("configure provider err = nil, want read-back invariant failure")
	}
}

func TestTestProviderUsesConfiguredProvider(t *testing.T) {
	probeErr := errors.New("bad key")
	rt := &providerRuntime{
		entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "sk-ant-secret"},
		},
		probeErr: probeErr,
	}
	s := newTestServer(rt)

	got, err := s.TestProvider(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("test provider: %v", err)
	}
	if got.OK || got.Error == nil || got.Error.Detail != probeErr.Error() {
		t.Fatalf("test result = %+v, want provider_test_failed", got)
	}
	if len(rt.probed) != 1 || rt.probed[0].ID != "anthropic" {
		t.Fatalf("probed = %+v, want anthropic", rt.probed)
	}
}
