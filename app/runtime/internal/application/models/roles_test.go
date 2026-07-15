package models

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
	"github.com/Tangerg/lynx/chatclient"
)

func TestSetUtilityRoleUsesSaverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(mustModelRole(t, "anthropic", "claude-haiku"))
	saver := &fakeUtilityRoleSaver{}
	c := New(Config{UtilityCell: cell, UtilityStore: saver})

	if err := c.SetUtilityRole(context.Background(), "anthropic", ""); err != nil {
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

func TestSetUtilityRoleUsesClientResolverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(&modelrole.Role{})
	saver := &fakeUtilityRoleSaver{}
	resolver := &fakeChatClientResolver{}
	c := New(Config{UtilityCell: cell, UtilityResolver: resolver, UtilityStore: saver})

	if err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); err != nil {
		t.Fatalf("SetUtilityRole err = %v", err)
	}

	if resolver.provider != "anthropic" || resolver.model != "claude-haiku" {
		t.Fatalf("resolver provider=%q model=%q", resolver.provider, resolver.model)
	}
	if saver.provider != "anthropic" || saver.model != "claude-haiku" {
		t.Fatalf("saved provider=%q model=%q", saver.provider, saver.model)
	}
}

func TestSetUtilityRoleReturnsClientResolverError(t *testing.T) {
	fail := errors.New("build client")
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(&modelrole.Role{})
	c := New(Config{UtilityCell: cell, UtilityResolver: &fakeChatClientResolver{err: fail}})

	if err := c.SetUtilityRole(context.Background(), "anthropic", "claude-haiku"); !errors.Is(err, fail) {
		t.Fatalf("SetUtilityRole err = %v, want %v", err, fail)
	}
}

func TestSetEmbeddingRoleUsesSaverPort(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	cell.Store(mustModelRole(t, "openai", "text-embedding-3-small"))
	saver := &fakeEmbeddingRoleSaver{}
	c := New(Config{EmbeddingCell: cell, EmbeddingStore: saver})

	if err := c.SetEmbeddingRole(context.Background(), "openai", ""); err != nil {
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

func TestSetUtilityRoleSerializesPersistAndPublish(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	saver := newBlockingUtilitySaver()
	c := New(Config{
		UtilityCell:     cell,
		UtilityResolver: staticClientResolver{},
		UtilityStore:    saver,
	})

	first := make(chan error, 1)
	go func() { first <- c.SetUtilityRole(t.Context(), "provider", "first") }()
	<-saver.firstStarted
	second := make(chan error, 1)
	go func() { second <- c.SetUtilityRole(t.Context(), "provider", "second") }()

	select {
	case <-saver.secondEntered:
		t.Fatal("second utility mutation entered persistence before the first published")
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
	if role := cell.Load(); role == nil || role.Model() != "second" {
		t.Fatalf("live role = %+v, want second", role)
	}
}

func TestSetEmbeddingRoleSerializesPersistAndPublish(t *testing.T) {
	cell := &atomic.Pointer[modelrole.Role]{}
	saver := newBlockingEmbeddingSaver()
	c := New(Config{
		EmbeddingCell:     cell,
		EmbeddingResolver: staticEmbeddingResolver{},
		EmbeddingStore:    saver,
	})

	first := make(chan error, 1)
	go func() { first <- c.SetEmbeddingRole(t.Context(), "provider", "first") }()
	<-saver.firstStarted
	second := make(chan error, 1)
	go func() { second <- c.SetEmbeddingRole(t.Context(), "provider", "second") }()

	select {
	case <-saver.secondEntered:
		t.Fatal("second embedding mutation entered persistence before the first published")
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
	if role := cell.Load(); role == nil || role.Model() != "second" {
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

type fakeChatClientResolver struct {
	provider string
	model    string
	err      error
}

func (r *fakeChatClientResolver) ResolveClient(_ context.Context, provider, model string) (*chatclient.Client, error) {
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

type staticClientResolver struct{}

func (staticClientResolver) ResolveClient(context.Context, string, string) (*chatclient.Client, error) {
	return nil, nil
}

type staticEmbeddingResolver struct{}

func (staticEmbeddingResolver) Resolve(context.Context, string, string) (codebaseindex.Embedder, error) {
	return staticEmbedder{}, nil
}

type staticEmbedder struct{}

func (staticEmbedder) ID() string { return "provider:model" }
func (staticEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
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
