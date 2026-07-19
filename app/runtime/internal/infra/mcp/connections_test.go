package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/core/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

type catalogTool string

func (t catalogTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: string(t), InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (catalogTool) Call(context.Context, string) (string, error) { return "", nil }

func TestConnectionsRejectMutationsAfterClose(t *testing.T) {
	c := &Connections{client: newClient()}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
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

	c.Remove(context.Background(), cfg.Name)
	if got := c.Statuses(); len(got) != 0 {
		t.Fatalf("statuses after Close + Remove = %v, want empty", got)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestConnectionsCloseSerializesWithMutations(t *testing.T) {
	c := &Connections{}
	c.reconnectMu.Lock()
	done := make(chan error, 1)
	go func() { done <- c.Close() }()

	select {
	case err := <-done:
		t.Fatalf("Close returned before the active mutation released its lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	c.reconnectMu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after the mutation lock was released")
	}
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
	c := &Connections{servers: []*server{
		{
			config:  ServerConfig{Name: "alpha"},
			session: new(sdkmcp.ClientSession),
			tools:   []tools.Tool{catalogTool("alpha_read"), catalogTool("alpha_list")},
		},
		{
			config:  ServerConfig{Name: "beta"},
			session: new(sdkmcp.ClientSession),
			tools:   []tools.Tool{catalogTool("beta_read")},
		},
	}}
	var got []string
	c.SetToolSink(func(catalog []tools.Tool) {
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

func TestRemovePublishesRemainingSnapshotWithCanceledContext(t *testing.T) {
	c := &Connections{servers: []*server{
		{config: ServerConfig{Name: "remove"}, tools: []tools.Tool{catalogTool("remove_read")}},
		{
			config:  ServerConfig{Name: "keep"},
			session: new(sdkmcp.ClientSession),
			tools:   []tools.Tool{catalogTool("keep_read")},
		},
	}}
	published := make(chan []string, 1)
	c.SetToolSink(func(catalog []tools.Tool) {
		names := make([]string, 0, len(catalog))
		for _, tool := range catalog {
			names = append(names, tool.Definition().Name)
		}
		published <- names
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c.Remove(ctx, "remove")
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
	c, initial, err := Dial(t.Context(), []ServerConfig{config})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if len(initial) != 1 || initial[0].Definition().Name != "remote_first" {
		t.Fatalf("initial tools = %v, want [remote_first]", toolNames(initial))
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []tools.Tool) { publications <- toolNames(catalog) })
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

func TestDialQuarantinesCrossServerPublicToolNameCollision(t *testing.T) {
	remote := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	addRemoteTool(t, remote, "read")
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return remote },
		nil,
	))
	t.Cleanup(httpServer.Close)

	c, initial, err := Dial(t.Context(), []ServerConfig{
		{Name: "a.b", Transport: TransportHTTP, Endpoint: httpServer.URL},
		{Name: "a_b", Transport: TransportHTTP, Endpoint: httpServer.URL},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_read"}) {
		t.Fatalf("initial tools = %v, want only the first server's tool", names)
	}
	statuses := c.Statuses()
	if len(statuses) != 2 || statuses[0].State != mcpserver.ConnectionConnected ||
		statuses[1].State != mcpserver.ConnectionFailed || statuses[1].Err == nil ||
		!strings.Contains(statuses[1].Err.Error(), `public tool name collision "a_b_read"`) {
		t.Fatalf("statuses = %+v, want connected then explicit collision failure", statuses)
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

	c, initial, err := Dial(t.Context(), []ServerConfig{{
		Name: "a_b", Transport: TransportHTTP, Endpoint: firstHTTP.URL,
	}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
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

	c, initial, err := Dial(t.Context(), []ServerConfig{
		{Name: "a_b", Transport: TransportHTTP, Endpoint: firstHTTP.URL},
		{Name: "a", Transport: TransportHTTP, Endpoint: secondHTTP.URL},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if names := toolNames(initial); !slices.Equal(names, []string{"a_b_c", "a_safe"}) {
		t.Fatalf("initial tools = %v, want [a_b_c a_safe]", names)
	}

	publications := make(chan []string, 2)
	c.SetToolSink(func(catalog []tools.Tool) { publications <- toolNames(catalog) })
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
		statuses[1].State != mcpserver.ConnectionFailed || statuses[1].Err == nil {
		t.Fatalf("statuses = %+v, want unaffected server connected and reconnected server failed", statuses)
	}
}

func addRemoteTool(t *testing.T, server *sdkmcp.Server, name string) {
	t.Helper()
	tool, err := tools.New[struct{}, string](tools.Config{Name: name}, func(context.Context, struct{}) (string, error) {
		return name, nil
	})
	if err != nil {
		t.Fatalf("build remote tool %q: %v", name, err)
	}
	if err := lynxmcp.Register(server, tool); err != nil {
		t.Fatalf("register remote tool %q: %v", name, err)
	}
}

func toolNames(catalog []tools.Tool) []string {
	names := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		names = append(names, tool.Definition().Name)
	}
	return names
}
