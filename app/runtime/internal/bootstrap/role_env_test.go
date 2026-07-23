package bootstrap

import (
	"context"
	"testing"
)

func TestLoadUtilityRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeUtilityRoleLoader{provider: "anthropic", model: "claude-haiku"}

	role, err := loadUtilityRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadUtilityRole err = %v", err)
	}

	if loader.calls != 1 || role.ProviderID() != "anthropic" || role.Model() != "claude-haiku" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

func TestLoadEmbeddingRoleUsesLoaderPort(t *testing.T) {
	loader := &fakeEmbeddingRoleLoader{provider: "openai", model: "text-embedding-3-small"}

	role, err := loadEmbeddingRole(context.Background(), loader)
	if err != nil {
		t.Fatalf("loadEmbeddingRole err = %v", err)
	}

	if loader.calls != 1 || role.ProviderID() != "openai" || role.Model() != "text-embedding-3-small" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

type fakeUtilityRoleLoader struct {
	provider string
	model    string
	calls    int
}

func (s *fakeUtilityRoleLoader) LoadUtilityRole(context.Context) (string, string, error) {
	s.calls++
	return s.provider, s.model, nil
}

type fakeEmbeddingRoleLoader struct {
	provider string
	model    string
	calls    int
}

func (s *fakeEmbeddingRoleLoader) LoadEmbeddingRole(context.Context) (string, string, error) {
	s.calls++
	return s.provider, s.model, nil
}
