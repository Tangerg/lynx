package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/tool"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

type echoInput struct {
	Text string `json:"text"`
}

// newEchoTool builds a minimal lynx Tool for tests.
func newEchoTool() tool.Tool {
	return testTool{
		definition: corechat.ToolDefinition{
			Name:        "echo",
			Description: "echo the input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		},
		call: func(_ context.Context, arguments string) (string, error) {
			var input echoInput
			if err := json.Unmarshal([]byte(arguments), &input); err != nil {
				return "", err
			}
			return input.Text, nil
		},
	}
}

func newConstantTool(name string) tool.Tool {
	return testTool{
		definition: corechat.ToolDefinition{
			Name:        name,
			Description: "return " + name,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		call: func(context.Context, string) (string, error) { return name, nil },
	}
}

type testTool struct {
	definition corechat.ToolDefinition
	call       func(context.Context, string) (string, error)
}

func (t testTool) Definition() corechat.ToolDefinition { return t.definition.Clone() }

func (t testTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.call(ctx, arguments)
}

// connectPair wires an in-memory MCP server (with the supplied lynx tools
// already registered) to a fresh client session, returning the live session
// and a cleanup func.
func connectPair(t *testing.T, ctx context.Context, registered ...tool.Tool) (*sdkmcp.ClientSession, func()) {
	t.Helper()
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "lynx-srv", Version: "v0.1.0"}, nil)
	require.NoError(t, lynxmcp.Register(srv, registered...))
	return connectServer(t, ctx, srv)
}

func connectServer(t *testing.T, ctx context.Context, srv *sdkmcp.Server) (*sdkmcp.ClientSession, func()) {
	t.Helper()
	srvT, cliT := sdkmcp.NewInMemoryTransports()

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
	ctx := t.Context()
	cs, cleanup := connectPair(t, ctx, newEchoTool())
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
	ctx := t.Context()

	failing := testTool{
		definition: corechat.ToolDefinition{
			Name:        "boom",
			Description: "always fails",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		call: func(context.Context, string) (string, error) {
			return "", errors.New("kaboom from lynx tool")
		},
	}

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
	require.ErrorIs(t, lynxmcp.Register(nil, newEchoTool()), lynxmcp.ErrNilServer)

	for _, test := range []struct {
		name string
		tool tool.Tool
	}{
		{name: "nil"},
		{name: "typed nil", tool: (*badSchemaTool)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := lynxmcp.Register(srv, test.tool)
			require.ErrorIs(t, err, tool.ErrInvalidTool)
		})
	}
}

func TestRegister_RejectsInvalidSchema(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
	// NewTool always derives a valid schema, so an invalid one can only reach
	// Register via a hand-rolled Tool — which is exactly what Register must reject.
	require.Error(t, lynxmcp.Register(srv, badSchemaTool{}))

	err := lynxmcp.Register(srv, missingSchemaTypeTool{})
	require.ErrorContains(t, err, `input schema type must be "object"`)
}

// The low-level AddTool path panics on a schema that does not declare an object, and
// Register hands the schema straight to it. What stops the panic is the Registry
// refusing the definition first, so every shape the server cannot hold is listed
// here against the one validator that refuses it — a second copy of this rule inside
// this module would be a second answer that drifts from the first.
func TestRegister_RefusesEverySchemaShapeTheServerCannotHold(t *testing.T) {
	for _, test := range []struct {
		name   string
		schema string
	}{
		{name: "absent", schema: ""},
		{name: "null", schema: `null`},
		{name: "not an object", schema: `[1,2]`},
		{name: "unparseable", schema: `{not-json`},
		{name: "no declared type", schema: `{}`},
		{name: "declares another type", schema: `{"type":"array"}`},
		{name: "type is not a string", schema: `{"type":123}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
			err := lynxmcp.Register(srv, testTool{definition: corechat.ToolDefinition{
				Name:        "shape",
				InputSchema: json.RawMessage(test.schema),
			}})
			// The sentinel is the point: the definition's own package owns what a valid
			// tool definition is, and the error says so all the way out to the caller.
			require.ErrorIs(t, err, corechat.ErrInvalidToolDefinition)
		})
	}
}

func TestRegister_RejectsDuplicateBatchAtomically(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
	err := lynxmcp.Register(srv, newConstantTool("duplicate"), newConstantTool("duplicate"))
	require.ErrorIs(t, err, tool.ErrDuplicateTool)

	require.NoError(t, lynxmcp.Register(srv, newConstantTool("after")))
	client, cleanup := connectServer(t, t.Context(), srv)
	defer cleanup()
	listed, err := client.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1)
	assert.Equal(t, "after", listed.Tools[0].Name)
}

func TestRegister_SnapshotsDefinitionOnce(t *testing.T) {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "x", Version: "v0"}, nil)
	tool := &definitionOnceTool{}
	require.NoError(t, lynxmcp.Register(srv, tool))

	client, cleanup := connectServer(t, t.Context(), srv)
	defer cleanup()
	result, err := client.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "stable",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.EqualValues(t, 1, tool.definitionCalls.Load())
}

// badSchemaTool is a tool.Tool whose InputSchema is not valid JSON, used to
// exercise Register's schema validation.
type badSchemaTool struct{}

func (badSchemaTool) Definition() corechat.ToolDefinition {
	return corechat.ToolDefinition{Name: "bad", InputSchema: json.RawMessage("{not-json")}
}

func (badSchemaTool) Call(context.Context, string) (string, error) { return "", nil }

type missingSchemaTypeTool struct{}

func (missingSchemaTypeTool) Definition() corechat.ToolDefinition {
	return corechat.ToolDefinition{Name: "missing-schema-type", InputSchema: json.RawMessage(`{}`)}
}

func (missingSchemaTypeTool) Call(context.Context, string) (string, error) { return "", nil }

type definitionOnceTool struct {
	definitionCalls atomic.Int32
}

func (t *definitionOnceTool) Definition() corechat.ToolDefinition {
	if t.definitionCalls.Add(1) != 1 {
		panic("tool definition was read after registration")
	}
	return corechat.ToolDefinition{
		Name:        "stable",
		Description: "stable definition",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (*definitionOnceTool) Call(context.Context, string) (string, error) { return "ok", nil }
