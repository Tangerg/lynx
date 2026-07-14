package bootstrap

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
)

func TestBuildUtilityEnvironmentUsesLoaderPort(t *testing.T) {
	client, err := chatclient.New(newReplyStub("ok"))
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	loader := &fakeUtilityRoleLoader{provider: "anthropic", model: "claude-haiku"}

	env, err := buildUtilityEnvironment(context.Background(), client, loader, nil)
	if err != nil {
		t.Fatalf("buildUtilityEnvironment err = %v", err)
	}

	role := env.cell.Load()
	if loader.calls != 1 || role == nil || role.ProviderID() != "anthropic" || role.Model() != "claude-haiku" {
		t.Fatalf("loaded calls=%d role=%+v", loader.calls, role)
	}
}

func TestBuildEmbeddingEnvironmentUsesLoaderPort(t *testing.T) {
	loader := &fakeEmbeddingRoleLoader{provider: "openai", model: "text-embedding-3-small"}

	env, err := buildEmbeddingEnvironment(context.Background(), loader, nil, nil)
	if err != nil {
		t.Fatalf("buildEmbeddingEnvironment err = %v", err)
	}

	role := env.cell.Load()
	if loader.calls != 1 || role == nil || role.ProviderID() != "openai" || role.Model() != "text-embedding-3-small" {
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
