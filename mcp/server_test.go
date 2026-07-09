package mcp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

type echoInput struct {
	Text string `json:"text"`
}

// newEchoTool builds a minimal lynx Tool for tests.
func newEchoTool(t *testing.T) chat.Tool {
	t.Helper()
	tool, err := chat.NewTool[echoInput, string](
		chat.ToolDefinition{
			Name:        "echo",
			Description: "echo the input",
		},
		func(_ context.Context, p echoInput) (string, error) { return p.Text, nil },
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

	failing, err := chat.NewTool[struct{}, string](
		chat.ToolDefinition{
			Name:        "boom",
			Description: "always fails",
		},
		func(context.Context, struct{}) (string, error) {
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
	// NewTool always derives a valid schema, so an invalid one can only reach
	// Register via a hand-rolled Tool — which is exactly what Register must reject.
	require.Error(t, lynxmcp.Register(srv, badSchemaTool{}))
}

// badSchemaTool is a chat.Tool whose InputSchema is not valid JSON, used to
// exercise Register's schema validation.
type badSchemaTool struct{}

func (badSchemaTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "bad", InputSchema: "{not-json"}
}

func (badSchemaTool) Call(context.Context, string) (string, error) { return "", nil }
