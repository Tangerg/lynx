package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

const echoSchema = `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`

// startServerWithEcho boots an in-memory MCP server that exposes a single
// "echo" tool (text -> text). It returns the live ClientSession the test
// should use to list tools, the underlying Server (so tests can mutate its
// tool list), and a cleanup that closes both sessions.
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

// TestToolsDefaultNamingSanitizesForProviderCharset locks the bridge that maps an
// MCP server/tool name onto the provider-accepted function-name charset
// (^[a-zA-Z0-9_-]{1,64}$). A server like "html.to.design" must NOT yield a
// dotted public name — that makes the whole chat request invalid and the
// provider rejects every turn.
func TestToolsDefaultNamingSanitizesForProviderCharset(t *testing.T) {
	cases := []struct {
		source string
		tool   string
		want   string
	}{
		{"html.to.design", "import-url", "html_to_design_import-url"},
		{"srv", "ok_tool-1", "srv_ok_tool-1"}, // already valid → unchanged
		{"", "bare.tool", "bare_tool"},        // empty source → sanitized bare name
		{"a b", "c/d", "a_b_c_d"},             // spaces + slash → underscores
	}
	for _, c := range cases {
		srvT, cliT := sdkmcp.NewInMemoryTransports()
		srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
		srv.AddTool(&sdkmcp.Tool{Name: c.tool, InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
		ss, err := srv.Connect(t.Context(), srvT, nil)
		require.NoError(t, err)
		defer ss.Close()

		cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
		cs, err := cli.Connect(t.Context(), cliT, nil)
		require.NoError(t, err)
		defer cs.Close()

		tools, err := lynxmcp.Tools(t.Context(), []lynxmcp.ToolSource{{Name: c.source, Session: cs}}, lynxmcp.ToolOptions{})
		require.NoError(t, err)
		require.Len(t, tools, 1)

		got := tools[0].Definition().Name
		assert.Equal(t, c.want, got)
		assert.Regexp(t, `^[a-zA-Z0-9_-]{1,64}$`, got)
	}
}

func TestToolsDiscoversAndCallsTool(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "primary", Session: cs}}, lynxmcp.ToolOptions{})
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

func TestToolsTwoSourcesAreNamespaced(t *testing.T) {
	ctx := context.Background()
	cs1, _, c1 := startServerWithEcho(t, ctx)
	defer c1()
	cs2, _, c2 := startServerWithEcho(t, ctx)
	defer c2()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{
		{Name: "alpha", Session: cs1},
		{Name: "beta", Session: cs2},
	}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, tools, 2)

	names := []string{tools[0].Definition().Name, tools[1].Definition().Name}
	assert.ElementsMatch(t, []string{"alpha_echo", "beta_echo"}, names)
}

func TestToolsFailsOnDuplicateNames(t *testing.T) {
	ctx := context.Background()
	cs1, _, c1 := startServerWithEcho(t, ctx)
	defer c1()
	cs2, _, c2 := startServerWithEcho(t, ctx)
	defer c2()

	_, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{
		{Name: "samename", Session: cs1},
		{Name: "samename", Session: cs2},
	}, lynxmcp.ToolOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool name")
}

func TestToolsCustomNaming(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "src", Session: cs}}, lynxmcp.ToolOptions{
		Naming: func(_ string, t *sdkmcp.Tool) string {
			return "mcp__" + t.Name
		},
	})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "mcp__echo", tools[0].Definition().Name)
}

func TestToolsRejectsEmptyPublicName(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	_, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "src", Session: cs}}, lynxmcp.ToolOptions{
		Naming: func(string, *sdkmcp.Tool) string { return "" },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "public name")
}

func TestToolsReadsCurrentRemoteList(t *testing.T) {
	ctx := context.Background()
	cs, srv, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	first, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "p", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, first, 1)

	srv.AddTool(
		&sdkmcp.Tool{Name: "ping", Description: "ping", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "pong"}}}, nil
		},
	)

	refreshed, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "p", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, refreshed, 2)

	names := []string{refreshed[0].Definition().Name, refreshed[1].Definition().Name}
	assert.ElementsMatch(t, []string{"p_echo", "p_ping"}, names)
}

func TestToolsRejectsNilSession(t *testing.T) {
	_, err := lynxmcp.Tools(context.Background(), []lynxmcp.ToolSource{{Name: "x"}}, lynxmcp.ToolOptions{})
	require.Error(t, err)
}

func TestToolsZeroOptionsUsesDefaults(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "primary", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "primary_echo", tools[0].Definition().Name, "default naming should join source and tool")
}
