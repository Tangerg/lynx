package mcp

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

func TestServersAndToolsUsePorts(t *testing.T) {
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 1}},
		tools:    []mcpserver.AdvertisedTool{{Server: "fs", Name: "read"}},
	}
	c := New(configWithPorts(ports))

	if got, err := c.Servers(context.Background()); err != nil || len(got) != 1 || got[0].Name != "fs" ||
		got[0].State.ToolCount == nil || *got[0].State.ToolCount != 1 {
		t.Fatalf("Servers = %+v, %v", got, err)
	}
	if ports.toolsCalls != 0 {
		t.Fatalf("status read made %d live tools/list calls, want 0", ports.toolsCalls)
	}
	tools, err := c.Tools(context.Background(), "fs")
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	if ports.toolsCalls != 1 || ports.toolsServer != "fs" || len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("tools server=%q tools=%+v", ports.toolsServer, tools)
	}
}

func TestDeleteServerPublishesRemovalAfterProjectionFailure(t *testing.T) {
	projectionErr := errors.New("projection detach failed")
	ports := &fakePorts{
		statuses:  []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected}},
		removeErr: projectionErr,
	}
	notified := make(chan string, 1)
	cfg := configWithPorts(ports)
	cfg.StatusChanged = func(status ServerStatus) { notified <- status.Name }
	c := New(cfg)

	if err := c.DeleteServer(t.Context(), "fs"); !errors.Is(err, projectionErr) {
		t.Fatalf("DeleteServer = %v, want projection failure", err)
	}
	if ports.removeName != "fs" {
		t.Fatalf("live removal = %q, want fs", ports.removeName)
	}
	if got := <-notified; got != "fs" {
		t.Fatalf("status notification = %q, want fs", got)
	}
}

// TestReconnectServerUsesPort: reconnect is fire-and-forget — it validates
// the name synchronously, then dials on the component task group and publishes
// the settled frame.
func TestReconnectServerUsesPort(t *testing.T) {
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected}}}
	settled := make(chan string, 1)
	cfg := configWithPorts(ports)
	cfg.StatusChanged = func(status ServerStatus) {
		if status.State != mcpserver.ConnectionConnecting {
			settled <- status.Name
		}
	}
	c := New(cfg)
	defer requireCoordinatorShutdown(t, c)

	if err := c.ReconnectServer(context.Background(), "fs"); err != nil {
		t.Fatalf("ReconnectServer err = %v", err)
	}
	if got := <-settled; got != "fs" {
		t.Fatalf("settled server = %q, want fs", got)
	}
	if ports.reconnectName != "fs" {
		t.Fatalf("reconnect=%q, want fs", ports.reconnectName)
	}

	if err := c.ReconnectServer(context.Background(), "ghost"); !errors.Is(err, ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownServer", err)
	}
}

func TestAuthorizationAttemptUsesPortAndSettles(t *testing.T) {
	authorizeStarted := make(chan string, 1)
	releaseAuthorize := make(chan struct{})
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: "github"}},
		authorizeStarted: authorizeStarted,
		releaseAuthorize: releaseAuthorize,
	}
	c := New(configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	attempt, err := c.CreateAuthorizationAttempt(context.Background(), "github")
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	if attempt.ID == "" || attempt.Server != "github" || attempt.Status != AuthorizationAttemptPending ||
		attempt.CreatedAt.IsZero() || attempt.FinishedAt != nil {
		t.Fatalf("created attempt = %+v", attempt)
	}
	if got := <-authorizeStarted; got != "github" {
		t.Fatalf("authorization target = %q, want github", got)
	}
	close(releaseAuthorize)

	settled := awaitAuthorizationAttempt(t, c, attempt.ID)
	if settled.Status != AuthorizationAttemptSucceeded || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want succeeded", settled)
	}
	if ports.authorizeName != "github" {
		t.Fatalf("authorize=%q, want github", ports.authorizeName)
	}
}

func TestConnectionValidationUsesDurableRegistry(t *testing.T) {
	const name = "fs"
	ports := &fakePorts{reconnectDone: make(chan string, 1)}
	registry := &testRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	c := New(Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	})

	// The live projection intentionally has no entry. Durable configuration is
	// the command authority; the background dial is what repairs that projection.
	if err := c.ReconnectServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectServer with stale live projection: %v", err)
	}
	if got := <-ports.reconnectDone; got != name {
		t.Fatalf("reconnect target = %q, want %q", got, name)
	}
	requireCoordinatorShutdown(t, c)
	if ports.reconnectName != name {
		t.Fatalf("reconnect target = %q, want %q", ports.reconnectName, name)
	}
}

func TestConnectionRejectsDurablyDisabledServer(t *testing.T) {
	const name = "fs"
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: false},
	}}
	c := New(Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	})

	// Even a stale connected projection cannot override the durable enablement
	// gate.
	if err := c.ReconnectServer(context.Background(), name); !errors.Is(err, ErrServerDisabled) {
		t.Fatalf("ReconnectServer = %v, want ErrServerDisabled", err)
	}
	if ports.reconnectName != "" {
		t.Fatalf("disabled server was dialed as %q", ports.reconnectName)
	}
}

func TestStatusCallbackMayReenterMutationWithoutDeadlock(t *testing.T) {
	const name = "fs"
	ports := &fakePorts{
		statuses: []mcpserver.ConnectionStatus{{Name: name, State: mcpserver.ConnectionConnected}},
	}
	registry := &testRegistry{servers: map[string]mcpserver.Server{
		name: {Name: name, Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-fs"},
	}}
	statuses := make(chan ServerStatus, 2)
	mutationResult := make(chan error, 1)
	cfg := Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
	var c *Coordinator
	cfg.StatusChanged = func(status ServerStatus) {
		statuses <- status
		if status.State == mcpserver.ConnectionConnecting {
			// A status consumer is application-external code. It may synchronously
			// issue another command; publication must hold neither mutation nor
			// delivery-ordering locks while invoking it.
			enabled := false
			_, err := c.UpdateServer(context.Background(), name, ServerPatch{Enabled: &enabled})
			mutationResult <- err
		}
	}
	c = New(cfg)

	if err := c.ReconnectServer(context.Background(), name); err != nil {
		t.Fatalf("ReconnectServer: %v", err)
	}
	first := <-statuses
	if first.Name != name || first.State != mcpserver.ConnectionConnecting || !first.Known {
		t.Fatalf("first status = %+v, want connecting", first)
	}
	if err := <-mutationResult; err != nil {
		t.Fatalf("reentrant UpdateServer: %v", err)
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

func TestConnectionRequiresCompleteDependencies(t *testing.T) {
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}}}
	c := New(Config{
		StatusReader:      ports,
		ConnectionControl: ports,
	})

	if err := c.ReconnectServer(context.Background(), "fs"); !errors.Is(err, errConnectionUnavailable) {
		t.Fatalf("ReconnectServer with incomplete dependencies = %v, want errConnectionUnavailable", err)
	}
}

// TestReconnectServerDetachedButComponentOwned: a dial detaches the caller's
// cancellation (a returning RPC must not abort it) while preserving its trace
// values, and is canceled + joined by Coordinator.Close; a reconnect requested
// after Close reports errClosed. This is the component-owned lifecycle §10.2/§10.3
// the delivery layer used to hold on its own task group.
func TestReconnectServerDetachedButComponentOwned(t *testing.T) {
	type ctxKey struct{}
	ports := &blockingPorts{
		fakePorts: fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: "fs"}}},
		started:   make(chan bool, 1),
		stopped:   make(chan struct{}),
		wantValue: func(ctx context.Context) bool { return ctx.Value(ctxKey{}) == "trace" },
	}
	c := New(configWithPorts(ports))

	reqCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace"))
	cancelRequest() // the request is done — the dial must keep running

	if err := c.ReconnectServer(reqCtx, "fs"); err != nil {
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
	if err := c.ReconnectServer(context.Background(), "fs"); !errors.Is(err, errClosed) {
		t.Fatalf("reconnect after Close = %v, want errClosed", err)
	}
}

func TestTestServerUsesLiveRegistryPort(t *testing.T) {
	ports := &fakePorts{}
	c := New(configWithPorts(ports))

	result, err := c.TestServer(context.Background(), ServerInput{
		Name: "fs", Connection: ConnectionInput{
			Transport: mcpserver.TransportStdio, Command: "mcp-fs",
			Args:        []string{"--root", "/repo"},
			Environment: &EnvironmentChange{Kind: SecretSet, Value: map[string]string{"A": "1"}},
		},
	})
	if err != nil {
		t.Fatalf("TestServer err = %v", err)
	}
	if !result.OK {
		t.Fatalf("TestServer result = %+v, want success", result)
	}
	if ports.probe.Name != "fs" || ports.probe.Command != "mcp-fs" || ports.probe.Env["A"] != "1" {
		t.Fatalf("probe config = %+v", ports.probe)
	}
}

type fakePorts struct {
	statuses []mcpserver.ConnectionStatus
	tools    []mcpserver.AdvertisedTool

	toolsServer string
	toolsCalls  int

	reconnectName    string
	reconnectDone    chan string
	authorizeName    string
	authorizeStarted chan string
	releaseAuthorize chan struct{}
	authorizeErr     error

	probe      mcpserver.Server
	configure  mcpserver.Server
	removeName string
	removeErr  error
}

func (f *fakePorts) Statuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakePorts) Tools(_ context.Context, server string) ([]mcpserver.AdvertisedTool, error) {
	f.toolsCalls++
	f.toolsServer = server
	return f.tools, nil
}

func (f *fakePorts) Reconnect(_ context.Context, name string) error {
	f.reconnectName = name
	if f.reconnectDone != nil {
		f.reconnectDone <- name
	}
	return nil
}

func (f *fakePorts) Authorize(ctx context.Context, name string) error {
	f.authorizeName = name
	if f.authorizeStarted != nil {
		f.authorizeStarted <- name
	}
	if f.releaseAuthorize != nil {
		select {
		case <-f.releaseAuthorize:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for index := range f.statuses {
		if f.statuses[index].Name != name {
			continue
		}
		if f.authorizeErr != nil {
			f.statuses[index].State = mcpserver.ConnectionFailed
		} else {
			f.statuses[index].State = mcpserver.ConnectionConnected
		}
	}
	return f.authorizeErr
}

func (f *fakePorts) Probe(_ context.Context, cfg mcpserver.Server) error {
	f.probe = cfg
	return nil
}

func (f *fakePorts) Configure(_ context.Context, cfg mcpserver.Server) error {
	f.configure = cfg
	return nil
}

func (f *fakePorts) Detach(name string) error {
	f.removeName = name
	return f.removeErr
}

// blockingPorts is a fakePorts whose dial blocks on its context until Close,
// so a test can observe the detach + component-owned-cancellation contract.
type blockingPorts struct {
	fakePorts
	started   chan bool
	stopped   chan struct{}
	wantValue func(context.Context) bool
}

func (f *blockingPorts) Reconnect(ctx context.Context, _ string) error {
	f.started <- ctx.Err() == nil && f.wantValue(ctx)
	<-ctx.Done()
	close(f.stopped)
	return ctx.Err()
}

func configWithPorts(ports interface {
	StatusReader
	ToolCatalog
	ConnectionControl
	ConnectionLifecycle
}) Config {
	registry := &testRegistry{servers: make(map[string]mcpserver.Server)}
	for _, status := range ports.Statuses() {
		registry.servers[status.Name] = mcpserver.Server{
			Name: status.Name, Enabled: true,
			Transport: mcpserver.TransportStreamableHTTP, URL: "https://mcp.example/" + status.Name,
		}
	}
	return Config{
		Registry:            registry,
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
}

func awaitAuthorizationAttempt(t *testing.T, c *Coordinator, id string) AuthorizationAttempt {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		attempt, err := c.AuthorizationAttempt(context.Background(), id)
		if err != nil {
			t.Fatalf("AuthorizationAttempt: %v", err)
		}
		if attempt.Status != AuthorizationAttemptPending {
			return attempt
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("MCP authorization attempt %q did not settle", id)
	return AuthorizationAttempt{}
}

// testRegistry is a concurrency-safe registry fake that preserves the
// domain Registry's sorted-list contract. Optional mutation hooks let
// concurrency tests stop a write after its durable commit.
type testRegistry struct {
	mu              sync.Mutex
	servers         map[string]mcpserver.Server
	saveCommitted   chan struct{}
	releaseSave     chan struct{}
	removeCommitted chan struct{}
	releaseRemove   chan struct{}
}

func (r *testRegistry) List(context.Context) ([]mcpserver.Server, error) {
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

func (r *testRegistry) Get(_ context.Context, name string) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	server, ok := r.servers[name]
	return server, ok, nil
}

func (r *testRegistry) Save(_ context.Context, server mcpserver.Server) error {
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

func (r *testRegistry) Remove(_ context.Context, name string) error {
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
