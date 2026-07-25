package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

func TestLoadUtilityRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeUtilityRoleLoader{role: mustBootstrapRole(t, "anthropic", "claude-haiku")}

	role, err := loadUtilityRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadUtilityRole err = %v", err)
	}

	if loader.calls != 1 || role.ProviderID() != "anthropic" || role.Model() != "claude-haiku" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

func TestLoadEmbeddingRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeEmbeddingRoleLoader{role: mustBootstrapRole(t, "openai", "text-embedding-3-small")}

	role, err := loadEmbeddingRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadEmbeddingRole err = %v", err)
	}

	if loader.calls != 1 || role.ProviderID() != "openai" || role.Model() != "text-embedding-3-small" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

type fakeUtilityRoleLoader struct {
	role  modelrole.Role
	calls int
}

func (s *fakeUtilityRoleLoader) LoadUtilityRole(context.Context) (modelrole.Role, error) {
	s.calls++
	return s.role, nil
}

type fakeEmbeddingRoleLoader struct {
	role  modelrole.Role
	calls int
}

func (s *fakeEmbeddingRoleLoader) LoadEmbeddingRole(context.Context) (modelrole.Role, error) {
	s.calls++
	return s.role, nil
}

func mustBootstrapRole(t testing.TB, provider, model string) modelrole.Role {
	t.Helper()
	role, err := modelrole.New(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}
