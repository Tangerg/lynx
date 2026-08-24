package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/core/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

type catalogTool string

type oauthHandlerStub struct{}

func (oauthHandlerStub) TokenSource(context.Context) (oauth2.TokenSource, error) { return nil, nil }
func (oauthHandlerStub) Authorize(context.Context, *http.Request, *http.Response) error {
	return nil
}

func TestReusableOAuthIsBoundToEndpointOrigin(t *testing.T) {
	handler := oauthHandlerStub{}
	current := ServerConfig{
		Name: "server", Transport: TransportHTTP, Endpoint: "https://EXAMPLE.com/mcp",
	}
	if got := reusableOAuth(current, ServerConfig{
		Name: "server", Transport: TransportHTTP, Endpoint: "https://example.com:443/other",
	}, handler); got == nil {
		t.Fatal("same-origin endpoint did not preserve OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Name: "server", Transport: TransportHTTP, Endpoint: "https://other.example/mcp",
	}, handler); got != nil {
		t.Fatal("cross-origin endpoint preserved OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Name: "server", Transport: TransportStdio, Command: "server",
	}, handler); got != nil {
		t.Fatal("transport change preserved OAuth handler")
	}
	if got := reusableOAuth(current, ServerConfig{
		Name: "server", Transport: TransportHTTP, Endpoint: "https://example.com/other",
		Authorization: "Bearer static",
	}, handler); got != nil {
		t.Fatal("static authorization preserved OAuth handler")
	}
}

func (t catalogTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: string(t), InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (catalogTool) Call(context.Context, string) (string, error) { return "", nil }

func TestConnectionsRejectMutationsAfterShutdown(t *testing.T) {
	c := &Connections{lifetime: t.Context(), client: newClient()}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	cfg := ServerConfig{Name: "closed", Transport: TransportHTTP, Endpoint: "https://example.invalid"}
	for name, call := range map[string]func() error{
		"configure": func() error { return c.Configure(context.Background(), cfg) },
		"reconnect": func() error { return c.Reconnect(context.Background(), cfg.Name) },
		"authorize": func() error { return c.Authorize(context.Background(), cfg.Name) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrConnectionsClosed) {
				t.Fatalf("error = %v, want ErrConnectionsClosed", err)
			}
		})
	}

	if err := c.Detach(cfg.Name); !errors.Is(err, ErrConnectionsClosed) {
		t.Fatalf("Detach after Shutdown = %v, want ErrConnectionsClosed", err)
	}
	if got := c.Statuses(); len(got) != 0 {
		t.Fatalf("statuses after Shutdown + Remove = %v, want empty", got)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestDialRequiresStartupAndProcessLifetimes(t *testing.T) {
	var missingContext context.Context
	if connections, _, err := Dial(missingContext, t.Context(), nil, nil); err == nil || connections != nil {
		t.Fatalf("Dial without startup context = (%v, %v), want nil connections and non-nil error", connections, err)
	}
	if connections, _, err := Dial(t.Context(), nil, nil, nil); err == nil || connections != nil {
		t.Fatalf("Dial without process lifetime = (%v, %v), want nil connections and non-nil error", connections, err)
	}
}

func TestNilConnectionsRejectRemoval(t *testing.T) {
	var connections *Connections
	if err := connections.Detach("server"); !errors.Is(err, ErrConnectionsUnavailable) {
		t.Fatalf("Detach on nil pool = %v, want ErrConnectionsUnavailable", err)
	}
}

func TestConnectionsShutdownCancelsAndJoinsAttempts(t *testing.T) {
	c := &Connections{lifetime: t.Context()}
	target := &server{config: ServerConfig{Name: "server"}}
	c.servers = []*server{target}
	c.mu.Lock()
	attempt := c.beginAttempt(t.Context(), target)
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := c.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Shutdown = %v, want context deadline exceeded", err)
	}
	select {
	case <-attempt.ctx.Done():
	default:
		t.Fatal("Shutdown did not cancel the active attempt")
	}

	c.finishAttempt(attempt)
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("join Shutdown: %v", err)
	}
}

func TestConnectionsShutdownSettlesTerminalSessionCloseError(t *testing.T) {
	closeErr := errors.New("session close failed")
	var calls atomic.Int32
	session := new(sdkmcp.ClientSession)
	owned := &ownedSession{
		// ClientSession.Close has this exact one-shot shape: its transport closer
		// is consumed even when it returns an error, so replay can only return the
		// same diagnostic and can never advance resource settlement.
		closeFn: sync.OnceValue(func() error {
			calls.Add(1)
			return closeErr
		}),
	}
	c := &Connections{lifetime: t.Context(),
		closed:   true,
		sessions: map[*sdkmcp.ClientSession]*ownedSession{session: owned},
	}

	if err := c.Shutdown(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("first Shutdown = %v, want close failure", err)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after terminal close error = %d, want 0", got)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown = %v, want settled no-op", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying session close calls = %d, want 1", got)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after repeated Shutdown = %d, want 0", got)
	}
}

func TestConnectionsShutdownReportsSettledAsyncRetirementDiagnosticOnce(t *testing.T) {
	closeErr := errors.New("retired session close failed")
	var calls atomic.Int32
	session := new(sdkmcp.ClientSession)
	c := &Connections{
		lifetime: t.Context(),
		sessions: map[*sdkmcp.ClientSession]*ownedSession{
			session: {
				closeFn: sync.OnceValue(func() error {
					calls.Add(1)
					return closeErr
				}),
			},
		},
	}

	c.retireSession(session)
	c.mu.Lock()
	var closeAttempt *sessionCloseAttempt
	for candidate := range c.retirements {
		closeAttempt = candidate
		break
	}
	c.mu.Unlock()
	if closeAttempt == nil {
		t.Fatal("asynchronous retirement was not registered")
	}
	<-closeAttempt.done
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after terminal retirement error = %d, want 0", got)
	}

	if err := c.Shutdown(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("first Shutdown = %v, want retirement diagnostic", err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown = %v, want settled no-op", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying session close calls = %d, want 1", got)
	}
}

func TestConnectionAttemptsSupersedePerServer(t *testing.T) {
	c := &Connections{lifetime: t.Context()}
	first := &server{config: ServerConfig{Name: "first"}}
	second := &server{config: ServerConfig{Name: "second"}}
	c.servers = []*server{first, second}

	c.mu.Lock()
	oldFirst := c.beginAttempt(t.Context(), first)
	secondAttempt := c.beginAttempt(t.Context(), second)
	newFirst := c.beginAttempt(t.Context(), first)
	if !c.currentAttempt(newFirst) || c.currentAttempt(oldFirst) == true {
		t.Fatal("latest first-server attempt did not own its generation")
	}
	c.mu.Unlock()

	if oldFirst.ctx.Err() == nil {
		t.Fatal("new same-server attempt did not cancel its predecessor")
	}
	if secondAttempt.ctx.Err() != nil {
		t.Fatal("first-server attempt canceled an unrelated server")
	}
	c.finishAttempt(oldFirst)
	c.finishAttempt(secondAttempt)
	c.finishAttempt(newFirst)
}

func TestCloneServerConfigOwnsMutableFields(t *testing.T) {
	original := ServerConfig{
		Args:    []string{"one"},
		Env:     []string{"A=1"},
		Headers: map[string]string{"X-Test": "before"},
	}
	cloned := cloneServerConfig(original)
	original.Args[0] = "two"
	original.Env[0] = "A=2"
	original.Headers["X-Test"] = "after"

	if cloned.Args[0] != "one" || cloned.Env[0] != "A=1" || cloned.Headers["X-Test"] != "before" {
		t.Fatalf("clone retained caller-owned storage: %+v", cloned)
	}
}

func TestPublishToolsUsesVerifiedSnapshotsInServerOrder(t *testing.T) {
	c := &Connections{lifetime: t.Context(), servers: []*server{
		{
			config:  ServerConfig{Name: "alpha"},
			session: new(sdkmcp.ClientSession),
			tools:   []toolcontract.Tool{catalogTool("alpha_read"), catalogTool("alpha_list")},
		},
		{
			config:  ServerConfig{Name: "beta"},
			session: new(sdkmcp.ClientSession),
			tools:   []toolcontract.Tool{catalogTool("beta_read")},
		},
	}}
	var got []string
	c.SetToolSink(func(catalog []toolcontract.Tool) {
		got = make([]string, 0, len(catalog))
		for _, tool := range catalog {
			got = append(got, tool.Definition().Name)
		}
	})

	c.publishTools()
	want := []string{"alpha_read", "alpha_list", "beta_read"}
	if !slices.Equal(got, want) {
		t.Fatalf("published tools = %v, want %v", got, want)
	}
}

func TestDetachPublishesRemainingSnapshot(t *testing.T) {
	c := &Connections{lifetime: t.Context(), servers: []*server{
		{config: ServerConfig{Name: "remove"}, tools: []toolcontract.Tool{catalogTool("remove_read")}},
		{
			config:  ServerConfig{Name: "keep"},
			session: new(sdkmcp.ClientSession),
			tools:   []toolcontract.Tool{catalogTool("keep_read")},
		},
	}}
	published := make(chan []string, 1)
	c.SetToolSink(func(catalog []toolcontract.Tool) {
		names := make([]string, 0, len(catalog))
		for _, tool := range catalog {
			names = append(names, tool.Definition().Name)
		}
		published <- names
	})
	if err := c.Detach("remove"); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if got := <-published; !slices.Equal(got, []string{"keep_read"}) {
		t.Fatalf("published tools = %v, want [keep_read]", got)
	}
}

func TestReconnectPublishesRemovalBeforeVerifiedReplacement(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "first")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	config := ServerConfig{Name: "remote", Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, initial, err := Dial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if len(initial) != 1 || initial[0].Definition().Name != "remote_first" {
		t.Fatalf("initial tools = %v, want [remote_first]", toolNames(initial))
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []toolcontract.Tool) { publications <- toolNames(catalog) })
	addRemoteTool(t, remote, "second")
	if err := c.Reconnect(t.Context(), config.Name); err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if connecting := <-publications; len(connecting) != 0 {
		t.Fatalf("connecting publication = %v, want empty", connecting)
	}
	settled := <-publications
	slices.Sort(settled)
	if want := []string{"remote_first", "remote_second"}; !slices.Equal(settled, want) {
		t.Fatalf("settled publication = %v, want %v", settled, want)
	}
}

func TestConfiguredSessionOutlivesRequestScope(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	connections, _, err := Dial(t.Context(), t.Context(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := connections.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	requestCtx, cancelRequest := context.WithCancel(t.Context())
	if err := connections.Configure(requestCtx, ServerConfig{
		Name: "dynamic", Transport: TransportHTTP, Endpoint: httpServer.URL,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cancelRequest()

	tools, err := connections.Tools(t.Context(), "dynamic")
	if err != nil {
		t.Fatalf("Tools after request scope ended: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("Tools after request scope ended = %+v, want dynamic/read", tools)
	}
}

func TestSessionLedgerOwnsReplacementUntilClose(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	config := ServerConfig{Name: "ledger", Transport: TransportHTTP, Endpoint: httpServer.URL}
	c, _, err := Dial(t.Context(), t.Context(), []ServerConfig{config}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 1 {
		t.Fatalf("owned sessions after Dial = %d, want 1", got)
	}
	if err := c.Reconnect(t.Context(), config.Name); err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 1 {
		t.Fatalf("owned sessions after replacement = %d, want 1", got)
	}
	if err := c.Detach(config.Name); err != nil {
		t.Fatal(err)
	}
	if err := c.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := ownedSessionCount(c); got != 0 {
		t.Fatalf("owned sessions after Detach + Shutdown = %d, want 0", got)
	}
}

func ownedSessionCount(c *Connections) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sessions)
}

func TestDialQuarantinesCrossServerPublicToolNameCollision(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	c, initial, err := Dial(t.Context(), t.Context(), []ServerConfig{
		{Name: "a.b", Transport: TransportHTTP, Endpoint: httpServer.URL},
		{Name: "a_b", Transport: TransportHTTP, Endpoint: httpServer.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_read"}) {
		t.Fatalf("initial tools = %v, want only the first server's tool", names)
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionFailed {
		t.Fatalf("statuses = %+v, want connected then failed", statuses)
	}
}

func TestConfigureRejectsCrossServerPublicToolNameCollision(t *testing.T) {
	firstRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "first", Version: "v1"}, nil)
	addRemoteTool(t, firstRemote, "c")
	firstHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return firstRemote },
		nil,
	))
	t.Cleanup(firstHTTP.Close)

	c, initial, err := Dial(t.Context(), t.Context(), []ServerConfig{{
		Name: "a_b", Transport: TransportHTTP, Endpoint: firstHTTP.URL,
	}}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_c"}) {
		t.Fatalf("initial tools = %v, want [a_b_c]", names)
	}

	secondRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "second", Version: "v1"}, nil)
	addRemoteTool(t, secondRemote, "b_c")
	secondHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return secondRemote },
		nil,
	))
	t.Cleanup(secondHTTP.Close)

	err = c.Configure(t.Context(), ServerConfig{Name: "a", Transport: TransportHTTP, Endpoint: secondHTTP.URL})
	if err == nil || !strings.Contains(err.Error(), `public tool name collision "a_b_c"`) {
		t.Fatalf("Configure collision error = %v", err)
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionFailed {
		t.Fatalf("statuses = %+v, want original connected and candidate failed", statuses)
	}
}

func TestReconnectQuarantinesNewCrossServerPublicToolNameCollision(t *testing.T) {
	firstRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "first", Version: "v1"}, nil)
	addRemoteTool(t, firstRemote, "c")
	firstHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return firstRemote },
		nil,
	))
	t.Cleanup(firstHTTP.Close)

	secondRemote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "second", Version: "v1"}, nil)
	addRemoteTool(t, secondRemote, "safe")
	secondHTTP := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return secondRemote },
		nil,
	))
	t.Cleanup(secondHTTP.Close)

	c, initial, err := Dial(t.Context(), t.Context(), []ServerConfig{
		{Name: "a_b", Transport: TransportHTTP, Endpoint: firstHTTP.URL},
		{Name: "a", Transport: TransportHTTP, Endpoint: secondHTTP.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_c", "a_safe"}) {
		t.Fatalf("initial tools = %v, want [a_b_c a_safe]", names)
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []toolcontract.Tool) { publications <- toolNames(catalog) })
	addRemoteTool(t, secondRemote, "b_c")
	err = c.Reconnect(t.Context(), "a")
	if err == nil || !strings.Contains(err.Error(), `public tool name collision "a_b_c"`) {
		t.Fatalf("Reconnect collision error = %v", err)
	}
	for phase := range 2 {
		if names := <-publications; !slices.Equal(names, []string{"a_b_c"}) {
			t.Fatalf("publication %d = %v, want only unaffected server", phase, names)
		}
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionFailed {
		t.Fatalf("statuses = %+v, want unaffected server connected and reconnected server failed", statuses)
	}
}

func addRemoteTool(t *testing.T, server *sdkmcp.Server, name string) {
	t.Helper()
	tool, err := toolcontract.NewFunc[struct{}, string](toolcontract.FuncConfig{Name: name}, func(context.Context, struct{}) (string, error) {
		return name, nil
	})
	if err != nil {
		t.Fatalf("build remote tool %q: %v", name, err)
	}
	if err := lynxmcp.Register(server, tool); err != nil {
		t.Fatalf("register remote tool %q: %v", name, err)
	}
}

func toolNames(catalog []toolcontract.Tool) []string {
	names := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		names = append(names, tool.Definition().Name)
	}
	return names
}
