package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

// liveSet is the registry-command projection the mutation tests observe.
type liveSet struct {
	mu         sync.Mutex
	servers    map[string]bool
	configured chan string
}

func (*liveSet) Probe(context.Context, mcpserver.Server) error {
	return nil
}

func (l *liveSet) Configure(ctx context.Context, cfg mcpserver.Server) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	l.servers[cfg.Name] = true
	l.mu.Unlock()
	if l.configured != nil {
		l.configured <- cfg.Name
	}
	return nil
}

func (l *liveSet) Detach(name string) error {
	l.mu.Lock()
	delete(l.servers, name)
	l.mu.Unlock()
	return nil
}

// blockingProjection deliberately has no adapter-local mutation lock. It
// proves the application coordinator, rather than a particular infrastructure
// implementation, owns reconnect/remove ordering.
type blockingProjection struct {
	liveSet
	name             string
	reconnectStarted chan struct{}
	releaseReconnect chan struct{}
}

func (b *blockingProjection) Statuses() []mcpserver.ConnectionStatus {
	return []mcpserver.ConnectionStatus{{Name: b.name}}
}

func (b *blockingProjection) Reconnect(ctx context.Context, name string) error {
	close(b.reconnectStarted)
	select {
	case <-b.releaseReconnect:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	b.servers[name] = true
	b.mu.Unlock()
	return nil
}

func (b *blockingProjection) Authorize(ctx context.Context, name string) error {
	return b.Reconnect(ctx, name)
}

func TestRegistryMutationIsLinearizedThroughLiveApply(t *testing.T) {
	registry := &testRegistry{
		servers:       map[string]mcpserver.Server{},
		saveCommitted: make(chan struct{}),
		releaseSave:   make(chan struct{}),
	}
	live := &liveSet{servers: map[string]bool{}}
	policy := mcpserver.NewToolPolicy(nil)
	c := New(Config{Registry: registry, ConnectionLifecycle: live, Policy: NewToolPolicyState(policy)})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}

	configured := make(chan error, 1)
	go func() {
		_, err := c.CreateServer(context.Background(), input(server))
		configured <- err
	}()
	<-registry.saveCommitted
	if c.mutationMu.TryLock() {
		c.mutationMu.Unlock()
		t.Fatal("configure released the mutation order before applying its live projection")
	}
	removed := make(chan error, 1)
	go func() { removed <- c.DeleteServer(context.Background(), server.Name) }()
	close(registry.releaseSave)
	if err := <-configured; err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if err := <-removed; err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}

	if _, ok, err := registry.Get(context.Background(), server.Name); err != nil || ok {
		t.Fatalf("registry final state: present=%v err=%v", ok, err)
	}
	live.mu.Lock()
	livePresent := live.servers[server.Name]
	live.mu.Unlock()
	if livePresent {
		t.Fatal("removed registry entry survived in the live MCP set")
	}
}

func TestPostCommitReconciliationOutlivesRequestCancellation(t *testing.T) {
	registry := &testRegistry{
		servers:       map[string]mcpserver.Server{},
		saveCommitted: make(chan struct{}),
		releaseSave:   make(chan struct{}),
	}
	live := &liveSet{servers: map[string]bool{}, configured: make(chan string, 1)}
	policy := mcpserver.NewToolPolicy(nil)
	c := New(Config{Registry: registry, ConnectionLifecycle: live, Policy: NewToolPolicyState(policy)})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.CreateServer(ctx, input(server))
		done <- err
	}()

	<-registry.saveCommitted
	cancel()
	close(registry.releaseSave)
	if err := <-done; err != nil {
		t.Fatalf("CreateServer after durable commit: %v", err)
	}
	if got := <-live.configured; got != server.Name {
		t.Fatalf("reconciled server = %q, want %q", got, server.Name)
	}
	requireCoordinatorShutdown(t, c)
	live.mu.Lock()
	livePresent := live.servers[server.Name]
	live.mu.Unlock()
	if !livePresent {
		t.Fatal("request cancellation abandoned post-commit live reconciliation")
	}
}

func input(server mcpserver.Server) ServerInput {
	var authorization *AuthorizationChange
	if server.Authorization != "" {
		authorization = &AuthorizationChange{Kind: SecretSet, Value: server.Authorization}
	}
	var headers *HeadersChange
	if len(server.Headers) > 0 {
		headers = &HeadersChange{Kind: SecretSet, Value: server.Headers}
	}
	var environment *EnvironmentChange
	if len(server.Env) > 0 {
		environment = &EnvironmentChange{Kind: SecretSet, Value: server.Env}
	}
	return ServerInput{
		Name: server.Name, Enabled: server.Enabled, Description: server.Description,
		Connection: ConnectionInput{
			Transport: server.Transport, URL: server.URL,
			Authorization: authorization, Headers: headers,
			Command: server.Command, Args: server.Args, Environment: environment, Dir: server.Dir,
		},
		Timeout: server.Timeout, DisabledTools: server.DisabledTools,
		AutoApproveTools: server.AutoApproveTools,
	}
}

func TestRemoveDoesNotWaitForInteractiveConnection(t *testing.T) {
	const name = "files"
	server := mcpserver.Server{Name: name, Enabled: true}
	registry := &testRegistry{servers: map[string]mcpserver.Server{name: server}}
	live := &blockingProjection{
		liveSet:          liveSet{servers: map[string]bool{name: true}},
		name:             name,
		reconnectStarted: make(chan struct{}),
		releaseReconnect: make(chan struct{}),
	}
	policy := mcpserver.NewToolPolicy([]mcpserver.Server{server})
	c := New(Config{
		Registry:            registry,
		StatusReader:        live,
		ConnectionControl:   live,
		ConnectionLifecycle: live,
		Policy:              NewToolPolicyState(policy),
	})
	defer requireCoordinatorShutdown(t, c)

	if err := c.ReconnectServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectServer: %v", err)
	}
	<-live.reconnectStarted

	removed := make(chan error, 1)
	go func() { removed <- c.DeleteServer(context.Background(), name) }()
	select {
	case err := <-removed:
		if err != nil {
			close(live.releaseReconnect)
			t.Fatalf("DeleteServer: %v", err)
		}
	case <-time.After(time.Second):
		close(live.releaseReconnect)
		t.Fatal("remove waited for the interactive connection")
	}
	close(live.releaseReconnect)
	requireCoordinatorShutdown(t, c) // joins the detached reconnect

	if _, ok, err := registry.Get(context.Background(), name); err != nil || ok {
		t.Fatalf("registry final state: present=%v err=%v", ok, err)
	}
	live.mu.Lock()
	livePresent := live.servers[name]
	live.mu.Unlock()
	if livePresent {
		t.Fatal("completed reconnect revived a durably removed MCP server")
	}
}

func TestQueuedReconnectCannotReviveRemovedServer(t *testing.T) {
	const name = "files"
	server := mcpserver.Server{Name: name, Enabled: true}
	registry := &testRegistry{
		servers:         map[string]mcpserver.Server{name: server},
		removeCommitted: make(chan struct{}),
		releaseRemove:   make(chan struct{}),
	}
	live := &blockingProjection{
		liveSet:          liveSet{servers: map[string]bool{name: true}},
		name:             name,
		reconnectStarted: make(chan struct{}),
		releaseReconnect: make(chan struct{}),
	}
	policy := mcpserver.NewToolPolicy([]mcpserver.Server{server})
	c := New(Config{
		Registry:            registry,
		StatusReader:        live,
		ConnectionControl:   live,
		ConnectionLifecycle: live,
		Policy:              NewToolPolicyState(policy),
	})

	removed := make(chan error, 1)
	go func() { removed <- c.DeleteServer(context.Background(), name) }()
	<-registry.removeCommitted
	if err := c.ReconnectServer(context.Background(), name); !errors.Is(err, ErrUnknownServer) {
		close(registry.releaseRemove)
		requireCoordinatorShutdown(t, c)
		t.Fatalf("ReconnectServer = %v, want ErrUnknownServer", err)
	}
	close(registry.releaseRemove)
	if err := <-removed; err != nil {
		requireCoordinatorShutdown(t, c)
		t.Fatalf("DeleteServer: %v", err)
	}
	requireCoordinatorShutdown(t, c)

	select {
	case <-live.reconnectStarted:
		t.Fatal("reconnect crossed a committed removal instead of reading the registry")
	default:
	}
	live.mu.Lock()
	livePresent := live.servers[name]
	live.mu.Unlock()
	if livePresent {
		t.Fatal("queued reconnect revived a durably removed MCP server")
	}
}
