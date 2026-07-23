package models

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func TestSetUtilityRoleUsesSaverPort(t *testing.T) {
	state := NewRoleState(*mustModelRole(t, "anthropic", "claude-haiku"))
	saver := &fakeUtilityRoleSaver{}
	c := New(Config{UtilityRoleState: state, UtilityStore: saver})

	if _, err := c.SetUtilityRole(context.Background(), "anthropic", ""); err != nil {
		t.Fatalf("SetUtilityRole err = %v", err)
	}

	role := state.Role()
	if saver.calls != 1 || saver.provider != "" || saver.model != "" {
		t.Fatalf("saved calls=%d provider=%q model=%q", saver.calls, saver.provider, saver.model)
	}
	if role.Configured() {
		t.Fatalf("role = %+v, want cleared", role)
	}
}

func TestSetUtilityRoleUsesChatModelValidatorPort(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	saver := &fakeUtilityRoleSaver{}
	validator := &fakeChatModelValidator{}
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = validator
	cfg.UtilityStore = saver
	c := New(cfg)

	if _, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); err != nil {
		t.Fatalf("SetUtilityRole err = %v", err)
	}

	if validator.provider != "anthropic" || validator.model != "claude-haiku" {
		t.Fatalf("validator provider=%q model=%q", validator.provider, validator.model)
	}
	if saver.provider != "anthropic" || saver.model != "claude-haiku" {
		t.Fatalf("saved provider=%q model=%q", saver.provider, saver.model)
	}
}

func TestSetUtilityRoleReturnsChatModelValidatorError(t *testing.T) {
	fail := errors.New("build client")
	state := NewRoleState(modelrole.Role{})
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = &fakeChatModelValidator{err: fail}
	c := New(cfg)

	if _, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); !errors.Is(err, fail) {
		t.Fatalf("SetUtilityRole err = %v, want %v", err, fail)
	}
}

func TestSetUtilityRoleRequiresChatModelValidator(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	c := New(cfg)

	_, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku")
	if err == nil || !strings.Contains(err.Error(), "validation is unavailable") {
		t.Fatalf("SetUtilityRole err = %v, want unavailable validation error", err)
	}
}

func TestSetUtilityRoleRequiresAConfiguredProvider(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	cfg := configuredRoleConfig()
	cfg.Providers = &testProviderRegistry{}
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = staticChatModelValidator{}
	c := New(cfg)

	_, err := c.SetUtilityRole(t.Context(), "anthropic", "claude-haiku")
	if !errors.Is(err, ErrProviderUnconfigured) {
		t.Fatalf("SetUtilityRole error = %v, want ErrProviderUnconfigured", err)
	}
}

func TestSetEmbeddingRoleUsesSaverPort(t *testing.T) {
	state := NewRoleState(*mustModelRole(t, "openai", "text-embedding-3-small"))
	saver := &fakeEmbeddingRoleSaver{}
	c := New(Config{EmbeddingRoleState: state, EmbeddingStore: saver})

	if _, err := c.SetEmbeddingRole(context.Background(), "openai", ""); err != nil {
		t.Fatalf("SetEmbeddingRole err = %v", err)
	}

	role := state.Role()
	if saver.calls != 1 || saver.provider != "" || saver.model != "" {
		t.Fatalf("saved calls=%d provider=%q model=%q", saver.calls, saver.provider, saver.model)
	}
	if role.Configured() {
		t.Fatalf("role = %+v, want cleared", role)
	}
}

func TestSetEmbeddingRoleRequiresResolver(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	cfg := configuredRoleConfig()
	cfg.EmbeddingRoleState = state
	c := New(cfg)

	_, err := c.SetEmbeddingRole(context.Background(), "openai", "text-embedding-3-small")
	if err == nil || !strings.Contains(err.Error(), "validation is unavailable") {
		t.Fatalf("SetEmbeddingRole err = %v, want unavailable validation error", err)
	}
}

func TestSetEmbeddingRoleRejectsProviderWithoutEmbeddings(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	cfg := configuredRoleConfig()
	cfg.Catalog = testCatalog{metadata: []provider.Metadata{{ID: "anthropic"}}}
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingResolver = staticEmbeddingResolver{}
	c := New(cfg)

	_, err := c.SetEmbeddingRole(t.Context(), "anthropic", "embedding")
	if !errors.Is(err, ErrEmbeddingUnsupported) {
		t.Fatalf("SetEmbeddingRole error = %v, want ErrEmbeddingUnsupported", err)
	}
}

func TestSetUtilityRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	saver := newBlockingUtilitySaver()
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = staticChatModelValidator{}
	cfg.UtilityStore = saver
	c := New(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetUtilityRole)
}

func TestSetEmbeddingRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelrole.Role{})
	saver := newBlockingEmbeddingSaver()
	cfg := configuredRoleConfig()
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingResolver = staticEmbeddingResolver{}
	cfg.EmbeddingStore = saver
	c := New(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetEmbeddingRole)
}

type roleMutation func(context.Context, string, string) (Role, error)

func assertRoleMutationSerializesPersistAndPublish(t *testing.T, state *RoleState, saver *blockingRoleSaver, setRole roleMutation) {
	t.Helper()
	first := make(chan error, 1)
	go func() { _, err := setRole(t.Context(), "provider", "first"); first <- err }()
	<-saver.firstStarted
	second := make(chan error, 1)
	go func() { _, err := setRole(t.Context(), "provider", "second"); second <- err }()
	select {
	case <-saver.secondEntered:
		t.Fatal("second mutation entered persistence before the first published")
	case <-time.After(20 * time.Millisecond):
	}
	close(saver.releaseFirst)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if got := saver.savedModel(); got != "second" {
		t.Fatalf("persisted model = %q, want second", got)
	}
	if role := state.Role(); role.Model() != "second" {
		t.Fatalf("live role = %+v, want second", role)
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

type fakeChatModelValidator struct {
	provider string
	model    string
	err      error
}

func (r *fakeChatModelValidator) ValidateChatModel(_ context.Context, provider, model string) error {
	r.provider = provider
	r.model = model
	return r.err
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

type staticChatModelValidator struct{}

func (staticChatModelValidator) ValidateChatModel(context.Context, string, string) error { return nil }

type staticEmbeddingResolver struct{}

func (staticEmbeddingResolver) Resolve(context.Context, string, string) (codebaseindex.Embedder, error) {
	return staticEmbedder{}, nil
}

type staticEmbedder struct{}

func (staticEmbedder) ID() string { return "provider:model" }
func (staticEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
}

func configuredRoleConfig() Config {
	return Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "key"},
			"openai":    {ID: "openai", APIKey: "key"},
			"provider":  {ID: "provider", APIKey: "key"},
		}},
		Catalog: testCatalog{metadata: []provider.Metadata{
			{ID: "anthropic", EmbeddingCapable: true},
			{ID: "openai", EmbeddingCapable: true},
			{ID: "provider", EmbeddingCapable: true},
		}},
	}
}

type blockingRoleSaver struct {
	firstStarted  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
	mu            sync.Mutex
	model         string
}

func newBlockingRoleSaver() *blockingRoleSaver {
	return &blockingRoleSaver{
		firstStarted:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (s *blockingRoleSaver) save(model string) {
	s.mu.Lock()
	s.model = model
	s.mu.Unlock()
	if model == "first" {
		close(s.firstStarted)
		<-s.releaseFirst
		return
	}
	close(s.secondEntered)
}

func (s *blockingRoleSaver) savedModel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

type blockingUtilitySaver struct{ *blockingRoleSaver }

func newBlockingUtilitySaver() *blockingUtilitySaver {
	return &blockingUtilitySaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (s *blockingUtilitySaver) SaveUtilityRole(_ context.Context, _, model string) error {
	s.save(model)
	return nil
}

type blockingEmbeddingSaver struct{ *blockingRoleSaver }

func newBlockingEmbeddingSaver() *blockingEmbeddingSaver {
	return &blockingEmbeddingSaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (s *blockingEmbeddingSaver) SaveEmbeddingRole(_ context.Context, _, model string) error {
	s.save(model)
	return nil
}
