package integrations

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

type blockingMCPRegistry struct {
	mu                 sync.Mutex
	servers            map[string]mcpserver.Server
	configureCommitted chan struct{}
	releaseConfigure   chan struct{}
}

func (r *blockingMCPRegistry) List(context.Context) ([]mcpserver.Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mcpserver.Server, 0, len(r.servers))
	for _, server := range r.servers {
		out = append(out, server)
	}
	return out, nil
}

func (r *blockingMCPRegistry) Get(_ context.Context, name string) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	server, ok := r.servers[name]
	return server, ok, nil
}

func (r *blockingMCPRegistry) Configure(_ context.Context, server mcpserver.Server) error {
	r.mu.Lock()
	r.servers[server.Name] = server
	r.mu.Unlock()
	close(r.configureCommitted)
	<-r.releaseConfigure
	return nil
}

func (r *blockingMCPRegistry) Remove(_ context.Context, name string) error {
	r.mu.Lock()
	delete(r.servers, name)
	r.mu.Unlock()
	return nil
}

func (r *blockingMCPRegistry) SetEnabled(_ context.Context, name string, enabled bool) error {
	r.mu.Lock()
	server := r.servers[name]
	server.Enabled = enabled
	r.servers[name] = server
	r.mu.Unlock()
	return nil
}

// mcpLiveSet is the MCPLive projection the registry-mutation tests observe. Only
// Configure/Remove are exercised; the read/connection legs are inert no-ops so
// the fake still satisfies the whole MCPLive port.
type mcpLiveSet struct {
	mu      sync.Mutex
	servers map[string]bool
}

func (*mcpLiveSet) MCPServerStatuses() []mcpserver.ConnectionStatus { return nil }
func (*mcpLiveSet) MCPTools(context.Context, string) ([]mcpserver.ToolInfo, error) {
	return nil, nil
}
func (*mcpLiveSet) ReconnectMCPServer(context.Context, string) error { return nil }
func (*mcpLiveSet) AuthorizeMCPServer(context.Context, string) error { return nil }
func (*mcpLiveSet) ProbeMCPServer(context.Context, mcpserver.LiveConfig) error {
	return nil
}

func (s *mcpLiveSet) ConfigureMCPServer(_ context.Context, cfg mcpserver.LiveConfig) error {
	s.mu.Lock()
	s.servers[cfg.Name] = true
	s.mu.Unlock()
	return nil
}

func (s *mcpLiveSet) RemoveMCPServer(_ context.Context, name string) {
	s.mu.Lock()
	delete(s.servers, name)
	s.mu.Unlock()
}

func TestMCPRegistryMutationIsLinearizedThroughLiveApply(t *testing.T) {
	registry := &blockingMCPRegistry{
		servers:            map[string]mcpserver.Server{},
		configureCommitted: make(chan struct{}),
		releaseConfigure:   make(chan struct{}),
	}
	live := &mcpLiveSet{servers: map[string]bool{}}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(nil)
	policyCell.Store(&policy)
	c := New(Config{MCPRegistry: registry, MCPLive: live, MCPPolicy: policyCell})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}

	configured := make(chan error, 1)
	go func() { configured <- c.ConfigureMCPServer(context.Background(), server) }()
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
	registry := &blockingMCPRegistry{
		servers:            map[string]mcpserver.Server{},
		configureCommitted: make(chan struct{}),
		releaseConfigure:   make(chan struct{}),
	}
	live := &mcpLiveSet{servers: map[string]bool{}}
	policyCell := &atomic.Pointer[mcpserver.ToolPolicy]{}
	policy := mcpserver.NewToolPolicy(nil)
	policyCell.Store(&policy)
	c := New(Config{MCPRegistry: registry, MCPLive: live, MCPPolicy: policyCell})
	server := mcpserver.Server{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.ConfigureMCPServer(ctx, server) }()

	<-registry.configureCommitted
	cancel()
	close(registry.releaseConfigure)
	if err := <-done; err != nil {
		t.Fatalf("ConfigureMCPServer after durable commit: %v", err)
	}
	live.mu.Lock()
	livePresent := live.servers[server.Name]
	live.mu.Unlock()
	if !livePresent {
		t.Fatal("request cancellation abandoned post-commit live reconciliation")
	}
}
