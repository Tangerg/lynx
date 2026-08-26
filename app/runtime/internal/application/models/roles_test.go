package models

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

func TestSetUtilityRoleUsesSaverPort(t *testing.T) {
	state := NewRoleState(*mustModelRole(t, "anthropic", "claude-haiku"))
	saver := &fakeUtilityRoleSaver{}
	c := New(Config{UtilityRoleState: state, UtilityStore: saver})

	if _, err := c.SetUtilityRole(context.Background(), "", ""); err != nil {
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
	state := NewRoleState(modelref.Selection{})
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
	state := NewRoleState(modelref.Selection{})
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = &fakeChatModelValidator{err: fail}
	c := New(cfg)

	if _, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); !errors.Is(err, fail) {
		t.Fatalf("SetUtilityRole err = %v, want %v", err, fail)
	}
}

func TestSetUtilityRoleRequiresChatModelValidator(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	c := New(cfg)

	_, err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku")
	if err == nil || !strings.Contains(err.Error(), "validation is unavailable") {
		t.Fatalf("SetUtilityRole err = %v, want unavailable validation error", err)
	}
}

func TestSetUtilityRoleRequiresAConfiguredProvider(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
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

	if _, err := c.SetEmbeddingRole(context.Background(), "", ""); err != nil {
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

func TestCommittedRoleUpdatesPublishModelsInvalidation(t *testing.T) {
	var notices []invalidation.Notice
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = NewRoleState(modelref.Selection{})
	cfg.UtilityValidator = staticChatModelValidator{}
	cfg.UtilityStore = &fakeUtilityRoleSaver{}
	cfg.EmbeddingRoleState = NewRoleState(modelref.Selection{})
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	cfg.EmbeddingStore = &fakeEmbeddingRoleSaver{}
	cfg.Invalidations = func(notice invalidation.Notice) { notices = append(notices, notice) }
	c := New(cfg)

	if _, err := c.SetUtilityRole(t.Context(), "anthropic", "claude-haiku"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetEmbeddingRole(t.Context(), "openai", "text-embedding-3-small"); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 2 {
		t.Fatalf("notices = %+v, want two", notices)
	}
	for _, notice := range notices {
		if notice.Resource != invalidation.Models {
			t.Fatalf("notice = %+v, want models", notice)
		}
	}
}

func TestSetEmbeddingRoleRequiresResolver(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
	cfg := configuredRoleConfig()
	cfg.EmbeddingRoleState = state
	c := New(cfg)

	_, err := c.SetEmbeddingRole(context.Background(), "openai", "text-embedding-3-small")
	if err == nil || !strings.Contains(err.Error(), "validation is unavailable") {
		t.Fatalf("SetEmbeddingRole err = %v, want unavailable validation error", err)
	}
}

func TestSetEmbeddingRoleRejectsProviderWithoutEmbeddings(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
	cfg := configuredRoleConfig()
	cfg.Catalog = testCatalog{metadata: []ProviderMetadata{{ID: "anthropic"}}}
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	c := New(cfg)

	_, err := c.SetEmbeddingRole(t.Context(), "anthropic", "embedding")
	if !errors.Is(err, ErrEmbeddingUnsupported) {
		t.Fatalf("SetEmbeddingRole error = %v, want ErrEmbeddingUnsupported", err)
	}
}

func TestSetUtilityRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
	saver := newBlockingUtilitySaver()
	cfg := configuredRoleConfig()
	cfg.UtilityRoleState = state
	cfg.UtilityValidator = staticChatModelValidator{}
	cfg.UtilityStore = saver
	c := New(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetUtilityRole)
}

func TestSetEmbeddingRoleSerializesPersistAndPublish(t *testing.T) {
	state := NewRoleState(modelref.Selection{})
	saver := newBlockingEmbeddingSaver()
	cfg := configuredRoleConfig()
	cfg.EmbeddingRoleState = state
	cfg.EmbeddingValidator = staticEmbeddingResolver{}
	cfg.EmbeddingStore = saver
	c := New(cfg)

	assertRoleMutationSerializesPersistAndPublish(t, state, saver.blockingRoleSaver, c.SetEmbeddingRole)
}

type roleMutation func(context.Context, string, string) (modelref.Selection, error)

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

func mustModelRole(t *testing.T, providerID, model string) *modelref.Selection {
	t.Helper()
	role, err := modelref.New(providerID, model)
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

func (f *fakeUtilityRoleSaver) SaveUtilityRole(_ context.Context, role modelref.Selection) error {
	f.calls++
	f.provider = role.Provider()
	f.model = role.Model()
	return nil
}

type fakeChatModelValidator struct {
	provider string
	model    string
	err      error
}

func (f *fakeChatModelValidator) ValidateChatModel(_ context.Context, provider, model string) error {
	f.provider = provider
	f.model = model
	return f.err
}

type fakeEmbeddingRoleSaver struct {
	provider string
	model    string
	calls    int
}

func (f *fakeEmbeddingRoleSaver) SaveEmbeddingRole(_ context.Context, role modelref.Selection) error {
	f.calls++
	f.provider = role.Provider()
	f.model = role.Model()
	return nil
}

type staticChatModelValidator struct{}

func (staticChatModelValidator) ValidateChatModel(context.Context, string, string) error { return nil }

type staticEmbeddingResolver struct{}

func (staticEmbeddingResolver) ValidateEmbeddingModel(context.Context, string, string) error {
	return nil
}

func configuredRoleConfig() Config {
	return Config{
		Providers: &testProviderRegistry{entries: map[string]provider.Provider{
			"anthropic": {ID: "anthropic", APIKey: "key"},
			"openai":    {ID: "openai", APIKey: "key"},
			"provider":  {ID: "provider", APIKey: "key"},
		}},
		Catalog: testCatalog{metadata: []ProviderMetadata{
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

func (b *blockingRoleSaver) save(model string) {
	b.mu.Lock()
	b.model = model
	b.mu.Unlock()
	if model == "first" {
		close(b.firstStarted)
		<-b.releaseFirst
		return
	}
	close(b.secondEntered)
}

func (b *blockingRoleSaver) savedModel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.model
}

type blockingUtilitySaver struct{ *blockingRoleSaver }

func newBlockingUtilitySaver() *blockingUtilitySaver {
	return &blockingUtilitySaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (b *blockingUtilitySaver) SaveUtilityRole(_ context.Context, role modelref.Selection) error {
	b.save(role.Model())
	return nil
}

type blockingEmbeddingSaver struct{ *blockingRoleSaver }

func newBlockingEmbeddingSaver() *blockingEmbeddingSaver {
	return &blockingEmbeddingSaver{blockingRoleSaver: newBlockingRoleSaver()}
}

func (b *blockingEmbeddingSaver) SaveEmbeddingRole(_ context.Context, role modelref.Selection) error {
	b.save(role.Model())
	return nil
}
