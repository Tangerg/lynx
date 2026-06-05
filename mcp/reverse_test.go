package mcp_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

// progressTool exercises ReportProgress + LogToClient from inside a
// tool body. ElicitFromClient needs a client-side handler, exercised
// in a dedicated test below.
type progressTool struct {
	calls *atomicInt
}

func (t *progressTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "progress_demo",
		Description: "Reports progress + log lines",
		InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
	}
}

func (t *progressTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *progressTool) Call(ctx context.Context, _ string) (string, error) {
	if mcp.ServerSessionFromContext(ctx) == nil {
		return "", errors.New("expected server session in ctx")
	}
	total := 3.0
	for i := 1; i <= 3; i++ {
		if err := mcp.ReportProgress(ctx, float64(i), &total, "step"); err != nil &&
			!errors.Is(err, mcp.ErrNoServerSession) {
			return "", err
		}
		_ = mcp.LogToClient(ctx, slog.LevelInfo, "step done", map[string]any{"i": i}, "demo")
	}
	t.calls.inc()
	return "ok", nil
}

func TestNotifyHelpers_ProgressAndLog(t *testing.T) {
	ctx := context.Background()

	// Capture progress notifications + log messages observed by the client.
	var (
		mu       sync.Mutex
		progress []sdkmcp.ProgressNotificationParams
		logLines []sdkmcp.LoggingMessageParams
	)

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-srv", Version: "v0.1.0"}, nil)
	require.NoError(t, mcp.RegisterTools(server, &progressTool{calls: &atomicInt{}}))

	cli := sdkmcp.NewClient(
		&sdkmcp.Implementation{Name: "test-cli", Version: "v0.1.0"},
		&sdkmcp.ClientOptions{
			ProgressNotificationHandler: func(_ context.Context, req *sdkmcp.ProgressNotificationClientRequest) {
				mu.Lock()
				progress = append(progress, *req.Params)
				mu.Unlock()
			},
			LoggingMessageHandler: func(_ context.Context, req *sdkmcp.LoggingMessageRequest) {
				mu.Lock()
				logLines = append(logLines, *req.Params)
				mu.Unlock()
			},
		},
	)

	srvT, cliT := sdkmcp.NewInMemoryTransports()
	srvSession, err := server.Connect(ctx, srvT, nil)
	require.NoError(t, err)
	defer srvSession.Close()

	cliSession, err := cli.Connect(ctx, cliT, nil)
	require.NoError(t, err)
	defer cliSession.Close()

	// Opt into log streaming (server stays silent until SetLevel).
	require.NoError(t, cliSession.SetLoggingLevel(ctx, &sdkmcp.SetLoggingLevelParams{Level: "debug"}))

	// Issue a tools/call with a progress token so progress notifications flow.
	params := &sdkmcp.CallToolParams{Name: "progress_demo"}
	params.SetProgressToken("p1")
	res, err := cliSession.CallTool(ctx, params)
	require.NoError(t, err)
	assert.False(t, res.IsError, "tool result: %#v", res)

	// Give the SDK a moment to deliver notifications (they go through
	// a goroutine on the client side). Close the sessions, which
	// flushes pending notifications.
	require.NoError(t, cliSession.Close())
	require.NoError(t, srvSession.Wait())

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, progress, 3, "expected 3 progress notifications")
	if assert.GreaterOrEqual(t, len(logLines), 1) {
		assert.Equal(t, sdkmcp.LoggingLevel("info"), logLines[0].Level)
	}
}

func TestNotifyHelpers_NoSessionReturnsSentinel(t *testing.T) {
	ctx := context.Background()
	err := mcp.ReportProgress(ctx, 1, nil, "no session")
	assert.ErrorIs(t, err, mcp.ErrNoServerSession)
	err = mcp.LogToClient(ctx, slog.LevelInfo, "no session", nil, "")
	assert.ErrorIs(t, err, mcp.ErrNoServerSession)
	_, err = mcp.ElicitFromClient(ctx, mcp.ElicitOptions{Message: "x"})
	assert.ErrorIs(t, err, mcp.ErrNoServerSession)
}

// atomicInt is a tiny goroutine-safe counter used by tests.
type atomicInt struct {
	mu sync.Mutex
	v  int
}

func (a *atomicInt) inc() {
	a.mu.Lock()
	a.v++
	a.mu.Unlock()
}
