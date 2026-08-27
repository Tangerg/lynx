package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

func TestLoadUtilityRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeUtilityRoleLoader{role: mustBootstrapRole(t, "anthropic", "claude-haiku")}

	role, err := loadUtilityRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadUtilityRole err = %v", err)
	}

	if loader.calls != 1 || role.Provider() != "anthropic" || role.Model() != "claude-haiku" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

func TestLoadEmbeddingRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeEmbeddingRoleLoader{role: mustBootstrapRole(t, "openai", "text-embedding-3-small")}

	role, err := loadEmbeddingRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadEmbeddingRole err = %v", err)
	}

	if loader.calls != 1 || role.Provider() != "openai" || role.Model() != "text-embedding-3-small" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

type fakeUtilityRoleLoader struct {
	role  modelref.Selection
	calls int
}

func (f *fakeUtilityRoleLoader) LoadUtilityRole(context.Context) (modelref.Selection, error) {
	f.calls++
	return f.role, nil
}

type fakeEmbeddingRoleLoader struct {
	role  modelref.Selection
	calls int
}

func (f *fakeEmbeddingRoleLoader) LoadEmbeddingRole(context.Context) (modelref.Selection, error) {
	f.calls++
	return f.role, nil
}

func mustBootstrapRole(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	role, err := modelref.New(provider, model)
	if err != nil {
		t.Fatal(err)
	}
	return role
}
