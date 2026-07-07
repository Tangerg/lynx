package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

type modelRoleRuntime struct {
	stubRuntime
	entries map[string]provider.Provider

	utilityProvider string
	utilityModel    string
	utilityCalls    int
}

func (r *modelRoleRuntime) ProviderMetadata(id string) (provider.Metadata, bool) {
	if id == "anthropic" {
		return provider.Metadata{ID: id}, true
	}
	return provider.Metadata{}, false
}

func (r *modelRoleRuntime) RegisteredProvider(_ context.Context, id string) (provider.Provider, bool, error) {
	entry, ok := r.entries[id]
	return entry, ok, nil
}

func (r *modelRoleRuntime) UtilityRole() (string, string) {
	return r.utilityProvider, r.utilityModel
}

func (r *modelRoleRuntime) SetUtilityRole(_ context.Context, providerID, model string) error {
	if model == "" {
		providerID = ""
	}
	r.utilityProvider = providerID
	r.utilityModel = model
	r.utilityCalls++
	return nil
}

func (r *modelRoleRuntime) EmbeddingRole() (string, string) { return "", "" }

func (r *modelRoleRuntime) SetEmbeddingRole(context.Context, string, string) error { return nil }

func TestSetUtilityRoleRequiresConfiguredProvider(t *testing.T) {
	rt := &modelRoleRuntime{entries: map[string]provider.Provider{}}
	s := &Server{runtimeBindings: runtimeBindings{providerRegistryCatalog: rt, providerCatalog: rt, utilityRole: rt}}

	_, err := s.SetUtilityRole(context.Background(), protocol.UtilityRole{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("set utility role err = %v, want ErrInvalidParams", err)
	}
	if rt.utilityCalls != 0 {
		t.Fatalf("utility role calls = %d, want 0", rt.utilityCalls)
	}
}

func TestSetUtilityRoleStoresConfiguredProvider(t *testing.T) {
	rt := &modelRoleRuntime{entries: map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-secret"},
	}}
	s := &Server{runtimeBindings: runtimeBindings{providerRegistryCatalog: rt, providerCatalog: rt, utilityRole: rt}}

	got, err := s.SetUtilityRole(context.Background(), protocol.UtilityRole{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
	})
	if err != nil {
		t.Fatalf("set utility role: %v", err)
	}
	if got.Provider != "anthropic" || got.Model != "claude-3-5-haiku-20241022" {
		t.Fatalf("utility role = %+v", got)
	}
	if rt.utilityCalls != 1 {
		t.Fatalf("utility role calls = %d, want 1", rt.utilityCalls)
	}
}
