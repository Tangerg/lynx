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

func TestMCPServersAndToolsUsePorts(t *testing.T) {
	ports := &fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 1}},
		tools:    []mcpserver.ToolInfo{{Server: "fs", Name: "read"}},
	}
	c := New(configWithMCPPorts(ports))

	if got, err := c.MCPServers(context.Background()); err != nil || len(got) != 1 || got[0].Name != "fs" ||
		got[0].State.ToolCount == nil || *got[0].State.ToolCount != 1 {
		t.Fatalf("MCPServers = %+v, %v", got, err)
	}
	if ports.toolsCalls != 0 {
		t.Fatalf("status read made %d live tools/list calls, want 0", ports.toolsCalls)
	}
	tools, err := c.MCPTools(context.Background(), "fs")
	if err != nil {
		t.Fatalf("MCPTools err = %v", err)
	}
	if ports.toolsCalls != 1 || ports.toolsServer != "fs" || len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("tools server=%q tools=%+v", ports.toolsServer, tools)
	}
}

func TestDeleteMCPServerPublishesRemovalAfterProjectionFailure(t *testing.T) {
	projectionErr := errors.New("projection detach failed")
	ports := &fakeMCPPorts{
		statuses:  []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected}},
		removeErr: projectionErr,
	}
	notified := make(chan string, 1)
	cfg := configWithMCPPorts(ports)
	cfg.MCPStatus = func(status MCPServerStatus) { notified <- status.Name }
	c := New(cfg)

	if err := c.DeleteMCPServer(t.Context(), "fs"); !errors.Is(err, projectionErr) {
		t.Fatalf("DeleteMCPServer = %v, want projection failure", err)
	}
	if ports.removeName != "fs" {
		t.Fatalf("live removal = %q, want fs", ports.removeName)
	}
	if got := <-notified; got != "fs" {
		t.Fatalf("status notification = %q, want fs", got)
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
	cfg.MCPStatus = func(status MCPServerStatus) {
		if status.State != mcpserver.ConnectionConnecting {
			settled <- status.Name
		}
	}
	c := New(cfg)
	defer requireCoordinatorShutdown(t, c)

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

	if err := c.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, ErrUnknownMCPServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownMCPServer", err)
	}
}

func TestMCPConnectionValidationUsesDurableRegistry(t *testing.T) {
	const name = "fs"
	ports := &fakeMCPPorts{reconnectDone: make(chan string, 1)}
	registry := &testMCPRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	c := New(Config{
		MCPRegistry:           registry,
		MCPStatusReader:       ports,
		MCPToolCatalog:        ports,
		MCPConnectionCommands: ports,
		MCPRegistryCommands:   ports,
	})

	// The live projection intentionally has no entry. Durable configuration is
	// the command authority; the background dial is what repairs that projection.
	if err := c.ReconnectMCPServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectMCPServer with stale live projection: %v", err)
	}
	if got := <-ports.reconnectDone; got != name {
		t.Fatalf("reconnect target = %q, want %q", got, name)
	}
	requireCoordinatorShutdown(t, c)
	if ports.reconnectName != name {
		t.Fatalf("reconnect target = %q, want %q", ports.reconnectName, name)
	}
}

func TestMCPConnectionRejectsDurablyDisabledServer(t *testing.T) {
	const name = "fs"
	ports := &fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testMCPRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: false},
	}}
	c := New(Config{
		MCPRegistry:           registry,
		MCPStatusReader:       ports,
		MCPToolCatalog:        ports,
		MCPConnectionCommands: ports,
		MCPRegistryCommands:   ports,
	})

	// Even a stale connected projection cannot override the durable enablement
	// gate.
	if err := c.ReconnectMCPServer(context.Background(), name); !errors.Is(err, ErrMCPServerDisabled) {
		t.Fatalf("ReconnectMCPServer = %v, want ErrMCPServerDisabled", err)
	}
	if ports.reconnectName != "" {
		t.Fatalf("disabled server was dialed as %q", ports.reconnectName)
	}
}

func TestMCPStatusCallbackMayReenterMutationWithoutDeadlock(t *testing.T) {
	const name = "fs"
	ports := &fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testMCPRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	statuses := make(chan MCPServerStatus, 2)
	mutationResult := make(chan error, 1)
	cfg := Config{
		MCPRegistry:           registry,
		MCPStatusReader:       ports,
		MCPToolCatalog:        ports,
		MCPConnectionCommands: ports,
		MCPRegistryCommands:   ports,
	}
	var c *Coordinator
	cfg.MCPStatus = func(status MCPServerStatus) {
		statuses <- status
		if status.State == mcpserver.ConnectionConnecting {
			// A status consumer is application-external code. It may synchronously
			// issue another command; publication must hold neither mutation nor
			// delivery-ordering locks while invoking it.
			enabled := false
			_, err := c.UpdateMCPServer(context.Background(), name, MCPServerPatch{Enabled: &enabled})
			mutationResult <- err
		}
	}
	c = New(cfg)

	if err := c.ReconnectMCPServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectMCPServer: %v", err)
	}
	first := <-statuses
	if first.Name != name || first.State != mcpserver.ConnectionConnecting || !first.Known {
		t.Fatalf("first status = %+v, want connecting", first)
	}
	if err := <-mutationResult; err != nil {
		t.Fatalf("reentrant UpdateMCPServer: %v", err)
	}
	second := <-statuses
	if second.Name != name || second.Known {
		t.Fatalf("second status = %+v, want ordered removal projection", second)
	}
	requireCoordinatorShutdown(t, c)

	server, ok, err := registry.Get(context.Background(), name)
	if err != nil || !ok || server.Enabled {
		t.Fatalf("durable server after reentrant disable = (%+v, %v, %v)", server, ok, err)
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

	requireCoordinatorShutdown(t, c)
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

	result, err := c.TestMCPServer(context.Background(), MCPServerInput{
		Name: "fs", Connection: MCPConnectionInput{
			Transport: mcpserver.TransportStdio, Command: "mcp-fs",
			Args:        []string{"--root", "/repo"},
			Environment: &MCPEnvironmentChange{Kind: MCPSecretSet, Value: map[string]string{"A": "1"}},
		},
	})
	if err != nil {
		t.Fatalf("TestMCPServer err = %v", err)
	}
	if !result.OK {
		t.Fatalf("TestMCPServer result = %+v, want success", result)
	}
	if ports.probe.Name != "fs" || ports.probe.Command != "mcp-fs" || ports.probe.Env["A"] != "1" {
		t.Fatalf("probe config = %+v", ports.probe)
	}
}

type fakeMCPPorts struct {
	statuses []mcpserver.ConnectionStatus
	tools    []mcpserver.ToolInfo

	toolsServer string
	toolsCalls  int

	reconnectName string
	reconnectDone chan string
	authorizeName string

	probe      mcpserver.Server
	configure  mcpserver.Server
	removeName string
	removeErr  error
}

func (f *fakeMCPPorts) Statuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakeMCPPorts) Tools(_ context.Context, server string) ([]mcpserver.ToolInfo, error) {
	f.toolsCalls++
	f.toolsServer = server
	return f.tools, nil
}

func (f *fakeMCPPorts) Reconnect(_ context.Context, name string) error {
	f.reconnectName = name
	if f.reconnectDone != nil {
		f.reconnectDone <- name
	}
	return nil
}

func (f *fakeMCPPorts) Authorize(_ context.Context, name string) error {
	f.authorizeName = name
	return nil
}

func (f *fakeMCPPorts) Probe(_ context.Context, cfg mcpserver.Server) error {
	f.probe = cfg
	return nil
}

func (f *fakeMCPPorts) Configure(_ context.Context, cfg mcpserver.Server) error {
	f.configure = cfg
	return nil
}

func (f *fakeMCPPorts) Detach(name string) error {
	f.removeName = name
	return f.removeErr
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
	mu              sync.Mutex
	servers         map[string]mcpserver.Server
	saveCommitted   chan struct{}
	releaseSave     chan struct{}
	removeCommitted chan struct{}
	releaseRemove   chan struct{}
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

func (r *testMCPRegistry) Save(_ context.Context, server mcpserver.Server) error {
	r.mu.Lock()
	r.servers[server.Name] = server
	r.mu.Unlock()
	if r.saveCommitted != nil {
		close(r.saveCommitted)
	}
	if r.releaseSave != nil {
		<-r.releaseSave
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
