package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

const echoSchema = `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`

// startServerWithEcho boots an in-memory MCP server that exposes a single
// "echo" tool (text -> text). It returns the live ClientSession the test
// should use to drive Provider, the underlying Server (so tests can mutate
// its tool list), and a cleanup that closes both sessions.
func startServerWithEcho(t *testing.T, ctx context.Context) (*sdkmcp.ClientSession, *sdkmcp.Server, func()) {
	t.Helper()
	srvT, cliT := sdkmcp.NewInMemoryTransports()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	srv.AddTool(
		&sdkmcp.Tool{
			Name:        "echo",
			Description: "echo the input",
			InputSchema: json.RawMessage(echoSchema),
		},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
				return &sdkmcp.CallToolResult{
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
					IsError: true,
				}, nil
			}
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: p.Text}},
			}, nil
		},
	)

	ss, err := srv.Connect(ctx, srvT, nil)
	require.NoError(t, err)

	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	cs, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)

	cleanup := func() {
		_ = cs.Close()
		_ = ss.Close()
	}
	return cs, srv, cleanup
}

func TestProvider_DiscoversAndCallsTool(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "primary", Session: cs}},
	})
	require.NoError(t, err)

	tools, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	def := tools[0].Definition()
	assert.Equal(t, "primary_echo", def.Name)
	assert.Equal(t, "echo the input", def.Description)
	assert.NotEmpty(t, def.InputSchema)

	callable := tools[0]
	out, err := callable.Call(ctx, `{"text":"hello world"}`)
	require.NoError(t, err)
	assert.Equal(t, "hello world", out)
}

func TestProvider_TwoSourcesAreNamespaced(t *testing.T) {
	ctx := context.Background()
	cs1, _, c1 := startServerWithEcho(t, ctx)
	defer c1()
	cs2, _, c2 := startServerWithEcho(t, ctx)
	defer c2()

	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{
			{Name: "alpha", Session: cs1},
			{Name: "beta", Session: cs2},
		},
	})
	require.NoError(t, err)

	tools, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 2)

	names := []string{tools[0].Definition().Name, tools[1].Definition().Name}
	assert.ElementsMatch(t, []string{"alpha_echo", "beta_echo"}, names)
}

func TestProvider_FailsOnDuplicateNames(t *testing.T) {
	ctx := context.Background()
	cs1, _, c1 := startServerWithEcho(t, ctx)
	defer c1()
	cs2, _, c2 := startServerWithEcho(t, ctx)
	defer c2()

	// Same source name => same prefix => duplicate "samename_echo"
	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{
			{Name: "samename", Session: cs1},
			{Name: "samename", Session: cs2},
		},
	})
	require.NoError(t, err)

	_, err = p.Tools(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func TestProvider_CustomNaming(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "src", Session: cs}},
		Naming: func(_ string, t *sdkmcp.Tool) string {
			return "mcp__" + t.Name
		},
	})
	require.NoError(t, err)

	tools, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "mcp__echo", tools[0].Definition().Name)
}

func TestProvider_CacheAndInvalidate(t *testing.T) {
	ctx := context.Background()
	cs, srv, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "p", Session: cs}},
	})
	require.NoError(t, err)

	first, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, first, 1)

	// Same call returns the cached slice (same backing array).
	second, err := p.Tools(ctx)
	require.NoError(t, err)
	assert.Same(t, &first[0], &second[0], "Tools() must return cached slice when not invalidated")

	// Add another tool on the server side. Without invalidation, the
	// provider must still report the old list.
	srv.AddTool(
		&sdkmcp.Tool{Name: "ping", Description: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong"}}}, nil
		},
	)
	stillCached, err := p.Tools(ctx)
	require.NoError(t, err)
	assert.Len(t, stillCached, 1, "stale cache should hide the new tool")

	// Invalidate -> next call must include the new tool.
	p.Invalidate()
	refreshed, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, refreshed, 2)

	names := []string{refreshed[0].Definition().Name, refreshed[1].Definition().Name}
	assert.ElementsMatch(t, []string{"p_echo", "p_ping"}, names)
}

func TestProvider_OnToolListChangedInvalidates(t *testing.T) {
	ctx := context.Background()

	srvT, cliT := sdkmcp.NewInMemoryTransports()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	srv.AddTool(
		&sdkmcp.Tool{Name: "echo", Description: "", InputSchema: json.RawMessage(echoSchema)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
		},
	)
	ss, err := srv.Connect(ctx, srvT, nil)
	require.NoError(t, err)
	defer ss.Close()

	// The handler captures `provider` by reference; we set it after Connect.
	// `notified` carries exactly one slot so the test can synchronize on
	// the SDK's async dispatcher.
	var provider *lynxmcp.Provider
	notified := make(chan struct{}, 1)

	cli := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"},
		&sdkmcp.ClientOptions{
			ToolListChangedHandler: func(ctx context.Context, req *sdkmcp.ToolListChangedRequest) {
				if provider != nil {
					provider.OnToolListChanged(ctx, req)
				}
				select {
				case notified <- struct{}{}:
				default:
				}
			},
		},
	)
	cs, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)
	defer cs.Close()

	provider, err = lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "p", Session: cs}},
	})
	require.NoError(t, err)

	first, err := provider.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, first, 1)

	// Adding a tool triggers tools/list_changed asynchronously.
	srv.AddTool(
		&sdkmcp.Tool{Name: "ping", Description: "", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong"}}}, nil
		},
	)

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive tools/list_changed notification within 2s")
	}

	refreshed, err := provider.Tools(ctx)
	require.NoError(t, err)
	assert.Len(t, refreshed, 2, "list_changed handler should have invalidated the cache")
}

func TestProvider_RejectsNilSession(t *testing.T) {
	_, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "x", Session: nil}},
	})
	require.Error(t, err)
}

func TestProvider_ZeroConfigUsesDefaults(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	p, err := lynxmcp.NewProvider(&lynxmcp.ProviderConfig{
		Sources: []lynxmcp.Source{{Name: "primary", Session: cs}},
	})
	require.NoError(t, err)

	tools, err := p.Tools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "primary_echo", tools[0].Definition().Name, "default naming should join source and tool")
}
