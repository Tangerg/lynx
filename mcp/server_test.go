package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

const lynxEchoSchema = `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`

// newEchoTool builds a minimal lynx Tool for tests.
func newEchoTool(t *testing.T) chat.Tool {
	t.Helper()
	tool, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "echo",
			Description: "echo the input",
			InputSchema: lynxEchoSchema,
		},
		func(ctx context.Context, args string) (string, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			return p.Text, nil
		},
	)
	require.NoError(t, err)
	return tool
}

// connectPair wires an in-memory MCP server (with the supplied lynx tools
// already registered) to a fresh client session, returning the live session
// and a cleanup func.
func connectPair(t *testing.T, ctx context.Context, tools ...chat.Tool) (*sdkmcp.ClientSession, func()) {
	t.Helper()
	srvT, cliT := sdkmcp.NewInMemoryTransports()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "lynx-srv", Version: "v0.1.0"}, nil)
	require.NoError(t, lynxmcp.Register(srv, tools...))

	ss, err := srv.Connect(ctx, srvT, nil)
	require.NoError(t, err)

	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-cli", Version: "v0.1.0"}, nil)
	cs, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)

	return cs, func() {
		_ = cs.Close()
		_ = ss.Close()
	}
}

func TestRegister_RoundTrip(t *testing.T) {
	ctx := context.Background()
	cs, cleanup := connectPair(t, ctx, newEchoTool(t))
	defer cleanup()

	list, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 1)
	assert.Equal(t, "echo", list.Tools[0].Name)
	assert.Equal(t, "echo the input", list.Tools[0].Description)

	// Schema arrived intact (decoded as map[string]any on the client side).
	schema, ok := list.Tools[0].InputSchema.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", schema["type"])

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"text": "round trip"},
	})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "round trip", tc.Text)
}

func TestRegister_ErrorBecomesIsError(t *testing.T) {
	ctx := context.Background()

	failing, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "boom",
			Description: "always fails",
			InputSchema: `{"type":"object"}`,
		},
		func(ctx context.Context, args string) (string, error) {
			return "", errors.New("kaboom from lynx tool")
		},
	)
	require.NoError(t, err)

	cs, cleanup := connectPair(t, ctx, failing)
	defer cleanup()

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "boom", Arguments: map[string]any{}})
	// Tool errors must NOT bubble up as protocol errors; they are reported
	// via IsError + TextContent so the LLM can self-correct.
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	tc := res.Content[0].(*sdkmcp.TextContent)
	assert.Contains(t, tc.Text, "kaboom from lynx tool")
}

func TestRegister_RejectsNilArgs(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
	require.Error(t, lynxmcp.Register(nil, newEchoTool(t)))

	err := lynxmcp.Register(srv, nil)
	require.Error(t, err)
}

func TestRegister_RejectsInvalidSchema(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
	bad, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "bad",
			Description: "",
			InputSchema: "{not-json",
		},
		func(ctx context.Context, args string) (string, error) { return "", nil },
	)
	require.NoError(t, err)

	err = lynxmcp.Register(srv, bad)
	require.Error(t, err)
}
