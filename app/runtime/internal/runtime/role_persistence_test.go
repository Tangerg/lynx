package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/core/model/chat"
)

func TestRuntimeSetUtilityRoleUsesSaverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(mustModelRole(t, "anthropic", "claude-haiku"))
	saver := &fakeUtilityRoleSaver{}
	rt := &Runtime{utility: cell, utilStore: saver}

	if err := rt.SetUtilityRole(context.Background(), "anthropic", ""); err != nil {
		t.Fatalf("SetUtilityRole err = %v", err)
	}

	role := cell.Load()
	if saver.calls != 1 || saver.provider != "" || saver.model != "" {
		t.Fatalf("saved calls=%d provider=%q model=%q", saver.calls, saver.provider, saver.model)
	}
	if role == nil || role.Configured() {
		t.Fatalf("role = %+v, want cleared", role)
	}
}

func TestRuntimeSetUtilityRoleUsesClientResolverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(&modelrole.Role{})
	saver := &fakeUtilityRoleSaver{}
	resolver := &fakeChatClientResolver{}
	rt := &Runtime{utility: cell, utilityClients: resolver, utilStore: saver}

	if err := rt.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); err != nil {
		t.Fatalf("SetUtilityRole err = %v", err)
	}

	if resolver.provider != "anthropic" || resolver.model != "claude-haiku" {
		t.Fatalf("resolver provider=%q model=%q", resolver.provider, resolver.model)
	}
	if saver.provider != "anthropic" || saver.model != "claude-haiku" {
		t.Fatalf("saved provider=%q model=%q", saver.provider, saver.model)
	}
}

func TestRuntimeSetUtilityRoleReturnsClientResolverError(t *testing.T) {
	fail := errors.New("build client")
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(&modelrole.Role{})
	rt := &Runtime{utility: cell, utilityClients: &fakeChatClientResolver{err: fail}}

	if err := rt.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); !errors.Is(err, fail) {
		t.Fatalf("SetUtilityRole err = %v, want %v", err, fail)
	}
}

func TestRuntimeSetEmbeddingRoleUsesSaverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(mustModelRole(t, "openai", "text-embedding-3-small"))
	saver := &fakeEmbeddingRoleSaver{}
	rt := &Runtime{embeddingCell: cell, embeddingStore: saver}

	if err := rt.SetEmbeddingRole(context.Background(), "openai", ""); err != nil {
		t.Fatalf("SetEmbeddingRole err = %v", err)
	}

	role := cell.Load()
	if saver.calls != 1 || saver.provider != "" || saver.model != "" {
		t.Fatalf("saved calls=%d provider=%q model=%q", saver.calls, saver.provider, saver.model)
	}
	if role == nil || role.Configured() {
		t.Fatalf("role = %+v, want cleared", role)
	}
}

func mustModelRole(t *testing.T, providerID, model string) *modelrole.Role {
	t.Helper()
	role, err := modelrole.New(providerID, model)
	if err != nil {
		t.Fatal(err)
	}
	return &role
}

type fakeUtilityRoleSaver struct {
	provider string
	model    string
	calls    int
}

func (s *fakeUtilityRoleSaver) SaveUtilityRole(_ context.Context, provider, model string) error {
	s.calls++
	s.provider = provider
	s.model = model
	return nil
}

type fakeChatClientResolver struct {
	provider string
	model    string
	err      error
}

func (r *fakeChatClientResolver) ResolveClient(_ context.Context, provider, model string) (*chat.Client, error) {
	r.provider = provider
	r.model = model
	if r.err != nil {
		return nil, r.err
	}
	return nil, nil
}

type fakeEmbeddingRoleSaver struct {
	provider string
	model    string
	calls    int
}

func (s *fakeEmbeddingRoleSaver) SaveEmbeddingRole(_ context.Context, provider, model string) error {
	s.calls++
	s.provider = provider
	s.model = model
	return nil
}
