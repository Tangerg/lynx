package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/models"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/provider"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// modelProviderFake satisfies the provider.Registry (Get) + ProviderCatalog
// (Metadata) surface the model-role validation reads.
type modelProviderFake struct{ entries map[string]provider.Provider }

func (m *modelProviderFake) List(context.Context) ([]provider.Provider, error) { return nil, nil }
func (m *modelProviderFake) Get(_ context.Context, id string) (provider.Provider, bool, error) {
	entry, ok := m.entries[id]
	return entry, ok, nil
}
func (m *modelProviderFake) Update(context.Context, string, provider.Patch) (provider.Provider, error) {
	return provider.Provider{}, nil
}
func (m *modelProviderFake) Supported() []models.ProviderMetadata {
	return []models.ProviderMetadata{{ID: "anthropic"}}
}
func (m *modelProviderFake) Metadata(id string) (models.ProviderMetadata, bool) {
	if id == "anthropic" {
		return models.ProviderMetadata{ID: id}, true
	}
	return models.ProviderMetadata{}, false
}
func (*modelProviderFake) Models(string) []models.Model { return nil }
func (*modelProviderFake) LookupModel(string, string) (models.Model, bool) {
	return models.Model{}, false
}

// okChatModelValidator always accepts the utility model.
type okChatModelValidator struct{}

func (okChatModelValidator) ValidateChatModel(context.Context, string, string) error { return nil }

type utilitySaverRecorder struct {
	provider string
	model    string
	calls    int
}

func (u *utilitySaverRecorder) SaveUtilityRole(_ context.Context, role modelref.Selection) error {
	u.calls++
	u.provider = role.Provider()
	u.model = role.Model()
	return nil
}

func modelRoleServer(entries map[string]provider.Provider, saver *utilitySaverRecorder) *Server {
	fake := &modelProviderFake{entries: entries}
	return serverWithModels(models.Config{
		Providers:        fake,
		Catalog:          fake,
		UtilityRoleState: models.NewRoleState(modelref.Selection{}),
		UtilityValidator: okChatModelValidator{},
		UtilityStore:     saver,
	})
}

func TestSetUtilityRoleRequiresConfiguredProvider(t *testing.T) {
	saver := &utilitySaverRecorder{}
	s := modelRoleServer(map[string]provider.Provider{}, saver)

	_, err := s.SetUtilityRole(context.Background(), protocol.UtilityRole{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("set utility role err = %v, want ErrInvalidParams", err)
	}
	if saver.calls != 0 {
		t.Fatalf("utility role calls = %d, want 0", saver.calls)
	}
}

func TestSetUtilityRoleRejectsPartialSelection(t *testing.T) {
	saver := &utilitySaverRecorder{}
	s := modelRoleServer(map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-secret"},
	}, saver)

	_, err := s.SetUtilityRole(context.Background(), protocol.UtilityRole{Provider: "anthropic"})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("set partial utility role err = %v, want ErrInvalidParams", err)
	}
	if saver.calls != 0 {
		t.Fatalf("utility role calls = %d, want 0", saver.calls)
	}
}

func TestSetUtilityRoleStoresConfiguredProvider(t *testing.T) {
	saver := &utilitySaverRecorder{}
	s := modelRoleServer(map[string]provider.Provider{
		"anthropic": {ID: "anthropic", APIKey: "sk-secret"},
	}, saver)

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
	if saver.calls != 1 {
		t.Fatalf("utility role calls = %d, want 1", saver.calls)
	}
}
