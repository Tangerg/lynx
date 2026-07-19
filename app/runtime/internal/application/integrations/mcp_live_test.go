package integrations

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestMCPStatusAndToolsUsePorts(t *testing.T) {
	ports := &fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected}},
		tools:    []mcpserver.ToolInfo{{Server: "fs", Name: "read"}},
	}
	c := New(configWithMCPPorts(ports))

	if got := c.MCPServerStatuses(); len(got) != 1 || got[0].Name != "fs" {
		t.Fatalf("MCPServerStatuses = %+v", got)
	}
	tools, err := c.MCPTools(context.Background(), "fs")
	if err != nil {
		t.Fatalf("MCPTools err = %v", err)
	}
	if ports.toolsServer != "fs" || len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("tools server=%q tools=%+v", ports.toolsServer, tools)
	}
}

// TestMCPConnectionCommandsUsePorts: reconnect/authorize are fire-and-forget —
// they validate the name synchronously, then dial on the component task group and
// publish the settled frame. The test waits on the settled notification (which
// runs after the dial) before asserting the live port was driven with the name.
func TestMCPConnectionCommandsUsePorts(t *testing.T) {
	ports := &fakeMCPPorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}, {Name: "github"}}}
	settled := make(chan string, 2)
	cfg := configWithMCPPorts(ports)
	cfg.MCPStatus = func(_ context.Context, server string, connecting bool) {
		if !connecting {
			settled <- server
		}
	}
	c := New(cfg)
	defer c.Close()

	if err := c.ReconnectMCPServer(context.Background(), "fs"); err != nil {
		t.Fatalf("ReconnectMCPServer err = %v", err)
	}
	if got := <-settled; got != "fs" {
		t.Fatalf("settled server = %q, want fs", got)
	}
	if err := c.AuthorizeMCPServer(context.Background(), "github"); err != nil {
		t.Fatalf("AuthorizeMCPServer err = %v", err)
	}
	if got := <-settled; got != "github" {
		t.Fatalf("settled server = %q, want github", got)
	}

	if ports.reconnectName != "fs" || ports.authorizeName != "github" {
		t.Fatalf("reconnect=%q authorize=%q", ports.reconnectName, ports.authorizeName)
	}

	if err := c.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, mcpserver.ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownServer", err)
	}
}

func TestMCPConnectionRequiresCompleteDependencies(t *testing.T) {
	ports := &fakeMCPPorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}}}
	c := New(Config{
		MCPStatusReader:       ports,
		MCPConnectionCommands: ports,
	})

	if err := c.ReconnectMCPServer(context.Background(), "fs"); !errors.Is(err, errMCPConnectionUnavailable) {
		t.Fatalf("ReconnectMCPServer with incomplete dependencies = %v, want errMCPConnectionUnavailable", err)
	}
}

// TestReconnectMCPServerDetachedButComponentOwned: a dial detaches the caller's
// cancellation (a returning RPC must not abort it) while preserving its trace
// values, and is canceled + joined by Coordinator.Close; a reconnect requested
// after Close reports errClosed. This is the component-owned lifecycle §10.2/§10.3
// the delivery layer used to hold on its own task group.
func TestReconnectMCPServerDetachedButComponentOwned(t *testing.T) {
	type ctxKey struct{}
	ports := &blockingMCPPorts{
		fakeMCPPorts: fakeMCPPorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}}},
		started:      make(chan bool, 1),
		stopped:      make(chan struct{}),
		wantValue:    func(ctx context.Context) bool { return ctx.Value(ctxKey{}) == "trace" },
	}
	c := New(configWithMCPPorts(ports))

	reqCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace"))
	cancelRequest() // the request is done — the dial must keep running

	if err := c.ReconnectMCPServer(reqCtx, "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if detached := <-ports.started; !detached {
		t.Fatal("dial context did not detach request cancellation or preserve values")
	}

	c.Close()
	select {
	case <-ports.stopped:
	case <-time.After(time.Second):
		t.Fatal("Coordinator.Close did not cancel and join the dial")
	}
	if err := c.ReconnectMCPServer(context.Background(), "fs"); !errors.Is(err, errClosed) {
		t.Fatalf("reconnect after Close = %v, want errClosed", err)
	}
}

func TestTestMCPServerUsesLiveRegistryPort(t *testing.T) {
	ports := &fakeMCPPorts{}
	c := New(configWithMCPPorts(ports))

	err := c.TestMCPServer(context.Background(), mcpserver.Server{
		Name:      "fs",
		Transport: mcpserver.TransportStdio,
		Command:   "mcp-fs",
		Args:      []string{"--root", "/repo"},
		Env:       map[string]string{"A": "1"},
	})
	if err != nil {
		t.Fatalf("TestMCPServer err = %v", err)
	}
	if ports.probe.Name != "fs" || ports.probe.Command != "mcp-fs" || len(ports.probe.Env) != 1 || ports.probe.Env[0] != "A=1" {
		t.Fatalf("probe config = %+v", ports.probe)
	}
}

type fakeMCPPorts struct {
	statuses []mcpserver.ConnectionStatus
	tools    []mcpserver.ToolInfo

	toolsServer string

	reconnectName string
	authorizeName string

	probe      mcpserver.LiveConfig
	configure  mcpserver.LiveConfig
	removeName string
}

func (f *fakeMCPPorts) Statuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakeMCPPorts) Tools(_ context.Context, server string) ([]mcpserver.ToolInfo, error) {
	f.toolsServer = server
	return f.tools, nil
}

func (f *fakeMCPPorts) Reconnect(_ context.Context, name string) error {
	f.reconnectName = name
	return nil
}

func (f *fakeMCPPorts) Authorize(_ context.Context, name string) error {
	f.authorizeName = name
	return nil
}

func (f *fakeMCPPorts) Probe(_ context.Context, cfg mcpserver.LiveConfig) error {
	f.probe = cfg
	return nil
}

func (f *fakeMCPPorts) Configure(_ context.Context, cfg mcpserver.LiveConfig) error {
	f.configure = cfg
	return nil
}

func (f *fakeMCPPorts) Remove(_ context.Context, name string) {
	f.removeName = name
}

// blockingMCPPorts is a fakeMCPPorts whose dial blocks on its context until Close,
// so a test can observe the detach + component-owned-cancellation contract.
type blockingMCPPorts struct {
	fakeMCPPorts
	started   chan bool
	stopped   chan struct{}
	wantValue func(context.Context) bool
}

func (f *blockingMCPPorts) Reconnect(ctx context.Context, _ string) error {
	f.started <- ctx.Err() == nil && f.wantValue(ctx)
	<-ctx.Done()
	close(f.stopped)
	return ctx.Err()
}

func configWithMCPPorts(ports interface {
	MCPStatusReader
	MCPToolCatalog
	MCPConnectionCommands
	MCPRegistryCommands
}) Config {
	registry := &testMCPRegistry{servers: make(map[string]mcpserver.Server)}
	for _, status := range ports.Statuses() {
		registry.servers[status.Name] = mcpserver.Server{Name: status.Name, Enabled: true}
	}
	return Config{
		MCPRegistry:           registry,
		MCPStatusReader:       ports,
		MCPToolCatalog:        ports,
		MCPConnectionCommands: ports,
		MCPRegistryCommands:   ports,
	}
}

// testMCPRegistry is a concurrency-safe registry fake that preserves the
// domain Registry's sorted-list contract. Optional mutation hooks let
// concurrency tests stop a write after its durable commit.
type testMCPRegistry struct {
	mu                 sync.Mutex
	servers            map[string]mcpserver.Server
	configureCommitted chan struct{}
	releaseConfigure   chan struct{}
	removeCommitted    chan struct{}
	releaseRemove      chan struct{}
}

func (r *testMCPRegistry) List(context.Context) ([]mcpserver.Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	servers := make([]mcpserver.Server, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}
	slices.SortFunc(servers, func(a, b mcpserver.Server) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return servers, nil
}

func (r *testMCPRegistry) Get(_ context.Context, name string) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	server, ok := r.servers[name]
	return server, ok, nil
}

func (r *testMCPRegistry) Configure(_ context.Context, server mcpserver.Server) error {
	r.mu.Lock()
	r.servers[server.Name] = server
	r.mu.Unlock()
	if r.configureCommitted != nil {
		close(r.configureCommitted)
	}
	if r.releaseConfigure != nil {
		<-r.releaseConfigure
	}
	return nil
}

func (r *testMCPRegistry) Remove(_ context.Context, name string) error {
	r.mu.Lock()
	delete(r.servers, name)
	r.mu.Unlock()
	if r.removeCommitted != nil {
		close(r.removeCommitted)
	}
	if r.releaseRemove != nil {
		<-r.releaseRemove
	}
	return nil
}

func (r *testMCPRegistry) SetEnabled(_ context.Context, name string, enabled bool) error {
	r.mu.Lock()
	if server, ok := r.servers[name]; ok {
		server.Enabled = enabled
		r.servers[name] = server
	}
	r.mu.Unlock()
	return nil
}
