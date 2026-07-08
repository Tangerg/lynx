package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

// startServerWithFailing exposes one tool that always returns IsError=true.
func startServerWithFailing(t *testing.T, ctx context.Context) (*sdkmcp.ClientSession, func()) {
	t.Helper()
	srvT, cliT := sdkmcp.NewInMemoryTransports()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "fail-srv", Version: "v0.1.0"}, nil)
	srv.AddTool(
		&sdkmcp.Tool{Name: "boom", Description: "always fails", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "kaboom"}},
				IsError: true,
			}, nil
		},
	)
	ss, err := srv.Connect(ctx, srvT, nil)
	require.NoError(t, err)

	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "fail-cli", Version: "v0.1.0"}, nil)
	cs, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)

	return cs, func() {
		_ = cs.Close()
		_ = ss.Close()
	}
}

func TestTool_IsErrorBecomesToolCallError(t *testing.T) {
	ctx := context.Background()
	cs, cleanup := startServerWithFailing(t, ctx)
	defer cleanup()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "s", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, tools, 1)

	callable := tools[0]
	out, err := callable.Call(ctx, "{}")
	require.Error(t, err)
	assert.Empty(t, out)

	// errors.AsType both classifies the error and exposes the structured payload.
	tcErr, ok := errors.AsType[*lynxmcp.ToolCallError](err)
	require.True(t, ok, "expected errors.AsType to extract *ToolCallError, got %v", err)
	assert.Equal(t, "boom", tcErr.ToolName)
	assert.Equal(t, "kaboom", tcErr.Message)
}

func TestTool_RPCErrorIsNotToolCallError(t *testing.T) {
	// Closing the session before a Call forces a transport error,
	// which must NOT be classified as *ToolCallError.
	ctx := context.Background()
	cs, cleanup := startServerWithFailing(t, ctx)
	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "s", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	cleanup() // close immediately

	_, callErr := tools[0].Call(ctx, "{}")
	require.Error(t, callErr)
	_, ok := errors.AsType[*lynxmcp.ToolCallError](callErr)
	assert.False(t, ok, "transport errors must not unwrap into *ToolCallError")
}

func TestTool_EmptyArgumentsTreatedAsEmptyObject(t *testing.T) {
	ctx := context.Background()
	cs, _, cleanup := startServerWithEcho(t, ctx)
	defer cleanup()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "s", Session: cs}}, lynxmcp.ToolOptions{})
	require.NoError(t, err)

	callable := tools[0]

	// echo without arguments — server returns empty string, no protocol error.
	out, err := callable.Call(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestTool_MetaForwardedToServer(t *testing.T) {
	ctx := context.Background()
	srvT, cliT := sdkmcp.NewInMemoryTransports()

	receivedMeta := make(chan map[string]any, 1)
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "meta-srv", Version: "v0.1.0"}, nil)
	srv.AddTool(
		&sdkmcp.Tool{Name: "snitch", Description: "reports meta", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			receivedMeta <- map[string]any(req.Params.Meta)
			return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "ok"}}}, nil
		},
	)
	ss, err := srv.Connect(ctx, srvT, nil)
	require.NoError(t, err)
	defer ss.Close()

	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "meta-cli", Version: "v0.1.0"}, nil)
	cs, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)
	defer cs.Close()

	tools, err := lynxmcp.Tools(ctx, []lynxmcp.ToolSource{{Name: "src", Session: cs}}, lynxmcp.ToolOptions{
		MetaFunc: lynxmcp.MetaFromContext,
	})
	require.NoError(t, err)

	callCtx := lynxmcp.WithMeta(ctx, sdkmcp.Meta{"userId": "u-42", "trace": "tx-99"})
	out, err := tools[0].Call(callCtx, "{}")
	require.NoError(t, err)
	assert.Equal(t, "ok", out)

	got := <-receivedMeta
	assert.Equal(t, "u-42", got["userId"])
	assert.Equal(t, "tx-99", got["trace"])
}
