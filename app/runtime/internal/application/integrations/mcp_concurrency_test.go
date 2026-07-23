package integrations

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// mcpLiveSet is the registry-command projection the mutation tests observe.
type mcpLiveSet struct {
	mu      sync.Mutex
	servers map[string]bool
}

func (*mcpLiveSet) Probe(context.Context, mcpserver.LiveConfig) error {
	return nil
}

func (s *mcpLiveSet) Configure(ctx context.Context, cfg mcpserver.LiveConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.servers[cfg.Name] = true
	s.mu.Unlock()
	return nil
}

func (s *mcpLiveSet) Remove(_ context.Context, name string) {
	s.mu.Lock()
	delete(s.servers, name)
	s.mu.Unlock()
}

// blockingMCPProjection deliberately has no adapter-local mutation lock. It
// proves the application coordinator, rather than a particular infrastructure
// implementation, owns reconnect/remove ordering.
type blockingMCPProjection struct {
	mcpLiveSet
	name             string
	reconnectStarted chan struct{}
	releaseReconnect chan struct{}
}

func (p *blockingMCPProjection) Statuses() []mcpserver.ConnectionStatus {
	return []mcpserver.ConnectionStatus{{Name: p.name}}
}

func (p *blockingMCPProjection) Reconnect(ctx context.Context, name string) error {
	close(p.reconnectStarted)
	select {
	case <-p.releaseReconnect:
	case <-ctx.Done():
		return ctx.Err()
	}
	p.mu.Lock()
	p.servers[name] = true
	p.mu.Unlock()
	return nil
}

func (p *blockingMCPProjection) Authorize(ctx context.Context, name string) error {
	return p.Reconnect(ctx, name)
}

func TestMCPRegistryMutationIsLinearizedThroughLiveApply(t *testing.T) {
	registry := &testMCPRegistry{
		servers:            map[string]mcpserver.Server{},
		configureCommitted: make(chan struct{}),
		releaseConfigure:   make(chan struct{}),
	}
	live := &mcpLiveSet{servers: map[string]bool{}}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(nil)
	policyCell.Store(&policy)
	c := New(Config{MCPRegistry: registry, MCPRegistryCommands: live, MCPPolicy: policyCell})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}

	configured := make(chan error, 1)
	go func() {
		_, err := c.ConfigureMCPServer(context.Background(), mcpInput(server))
		configured <- err
	}()
	<-registry.configureCommitted
	removed := make(chan error, 1)
	go func() { removed <- c.RemoveMCPServer(context.Background(), server.Name) }()
	select {
	case err := <-removed:
		t.Fatalf("remove crossed an incomplete configure workflow: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(registry.releaseConfigure)
	if err := <-configured; err != nil {
		t.Fatalf("ConfigureMCPServer: %v", err)
	}
	if err := <-removed; err != nil {
		t.Fatalf("RemoveMCPServer: %v", err)
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

func TestMCPPostCommitReconciliationOutlivesRequestCancellation(t *testing.T) {
	registry := &testMCPRegistry{
		servers:            map[string]mcpserver.Server{},
		configureCommitted: make(chan struct{}),
		releaseConfigure:   make(chan struct{}),
	}
	live := &mcpLiveSet{servers: map[string]bool{}}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(nil)
	policyCell.Store(&policy)
	c := New(Config{MCPRegistry: registry, MCPRegistryCommands: live, MCPPolicy: policyCell})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.ConfigureMCPServer(ctx, mcpInput(server))
		done <- err
	}()

	<-registry.configureCommitted
	cancel()
	close(registry.releaseConfigure)
	if err := <-done; err != nil {
		t.Fatalf("ConfigureMCPServer after durable commit: %v", err)
	}
	// The live (re)dial is dispatched detached so a slow handshake never holds the
	// mutation lock, so it settles shortly after the call returns rather than
	// synchronously. It must still run despite the request-ctx cancellation, since
	// it is dispatched on the owner-scoped task group.
	deadline := time.After(time.Second)
	for {
		live.mu.Lock()
		livePresent := live.servers[server.Name]
		live.mu.Unlock()
		if livePresent {
			return
		}
		select {
		case <-deadline:
			t.Fatal("request cancellation abandoned post-commit live reconciliation")
		case <-time.After(time.Millisecond):
		}
	}
}

func mcpInput(server mcpserver.Server) MCPServerInput {
	return MCPServerInput{
		Name: server.Name, Transport: string(server.Transport), Enabled: server.Enabled,
		Description: server.Description, URL: server.URL, Authorization: server.Authorization,
		Headers: server.Headers, Command: server.Command, Args: server.Args, Env: server.Env,
		Dir: server.Dir, Timeout: server.Timeout, DisabledTools: server.DisabledTools,
		AutoApproveTools: server.AutoApproveTools,
	}
}

func TestMCPRemoveDoesNotWaitForInteractiveConnection(t *testing.T) {
	const name = "files"
	server := mcpserver.Server{Name: name, Enabled: true}
	registry := &testMCPRegistry{servers: map[string]mcpserver.Server{name: server}}
	live := &blockingMCPProjection{
		mcpLiveSet:       mcpLiveSet{servers: map[string]bool{name: true}},
		name:             name,
		reconnectStarted: make(chan struct{}),
		releaseReconnect: make(chan struct{}),
	}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy([]mcpserver.Server{server})
	policyCell.Store(&policy)
	c := New(Config{
		MCPRegistry:           registry,
		MCPStatusReader:       live,
		MCPConnectionCommands: live,
		MCPRegistryCommands:   live,
		MCPPolicy:             policyCell,
	})
	defer c.Close()

	if err := c.ReconnectMCPServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectMCPServer: %v", err)
	}
	<-live.reconnectStarted

	removed := make(chan error, 1)
	go func() { removed <- c.RemoveMCPServer(context.Background(), name) }()
	select {
	case err := <-removed:
		if err != nil {
			close(live.releaseReconnect)
			t.Fatalf("RemoveMCPServer: %v", err)
		}
	case <-time.After(time.Second):
		close(live.releaseReconnect)
		t.Fatal("remove waited for the interactive connection")
	}
	close(live.releaseReconnect)
	c.Close() // joins the detached reconnect and its stale-projection cleanup

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

func TestMCPQueuedReconnectCannotReviveRemovedServer(t *testing.T) {
	const name = "files"
	server := mcpserver.Server{Name: name, Enabled: true}
	registry := &testMCPRegistry{
		servers:         map[string]mcpserver.Server{name: server},
		removeCommitted: make(chan struct{}),
		releaseRemove:   make(chan struct{}),
	}
	live := &blockingMCPProjection{
		mcpLiveSet:       mcpLiveSet{servers: map[string]bool{name: true}},
		name:             name,
		reconnectStarted: make(chan struct{}),
		releaseReconnect: make(chan struct{}),
	}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy([]mcpserver.Server{server})
	policyCell.Store(&policy)
	c := New(Config{
		MCPRegistry:           registry,
		MCPStatusReader:       live,
		MCPConnectionCommands: live,
		MCPRegistryCommands:   live,
		MCPPolicy:             policyCell,
	})

	removed := make(chan error, 1)
	go func() { removed <- c.RemoveMCPServer(context.Background(), name) }()
	<-registry.removeCommitted
	if err := c.ReconnectMCPServer(context.Background(), name); err != nil {
		close(registry.releaseRemove)
		close(live.releaseReconnect)
		c.Close()
		t.Fatalf("ReconnectMCPServer: %v", err)
	}

	reconnectStarted := false
	select {
	case <-live.reconnectStarted:
		reconnectStarted = true
	case <-time.After(20 * time.Millisecond):
	}
	close(registry.releaseRemove)
	if err := <-removed; err != nil {
		close(live.releaseReconnect)
		c.Close()
		t.Fatalf("RemoveMCPServer: %v", err)
	}
	close(live.releaseReconnect)
	c.Close()

	if reconnectStarted {
		t.Fatal("reconnect crossed a committed removal instead of revalidating the registry")
	}
	live.mu.Lock()
	livePresent := live.servers[name]
	live.mu.Unlock()
	if livePresent {
		t.Fatal("queued reconnect revived a durably removed MCP server")
	}
}
