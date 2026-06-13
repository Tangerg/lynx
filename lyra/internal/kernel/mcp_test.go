package kernel

import (
	"context"
	"errors"
	"log"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"

	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset"
)

// runAsMCPServerEnv is the env-var sentinel that flips this test
// binary into "act as a stdio MCP server" mode. The stdio integration
// test (below) re-execs the test binary with this set so we get a
// real subprocess to talk to over stdin/stdout without depending on
// `npx` / Python / etc. in CI.
const runAsMCPServerEnv = "LYRA_TEST_RUN_AS_MCP_SERVER"

// TestMain is the standard fork-and-exec trick documented in the
// SDK's cmd_test.go: when LYRA_TEST_RUN_AS_MCP_SERVER is set, run as
// a stdio MCP server (with one `ping` tool) instead of executing
// the test suite.
func TestMain(m *testing.M) {
	if os.Getenv(runAsMCPServerEnv) != "" {
		runStdioMCPServer()
		return
	}
	os.Exit(m.Run())
}

// runStdioMCPServer is the entry point used when the test binary
// re-execs itself as an MCP server. Mirrors the structure of the
// HTTP test server: one `ping` tool, stdio transport.
func runStdioMCPServer() {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "lyra-test-stdio-mcp", Version: "v0.1.0",
	}, nil)
	ping, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "ping",
			Description: "responds with pong",
			InputSchema: `{"type":"object"}`,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, args string) (string, error) { return "pong", nil },
	)
	if err != nil {
		log.Fatalf("build tool: %v", err)
	}
	if err := mcp.RegisterTools(srv, ping); err != nil {
		log.Fatalf("register tools: %v", err)
	}
	transport := &sdkmcp.StdioTransport{}
	if err := srv.Run(context.Background(), transport); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// TestEngine_DialMCPServer brings up an in-process MCP server
// over HTTP, registers one tool against it, then constructs an
// Engine wired to dial the server. The engine's tool list must
// include the remote tool under its prefixed name; Close must
// drop the session cleanly.
func TestEngine_DialMCPServer(t *testing.T) {
	// 1. Spin up a real MCP server with one tool.
	mcpServer := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "test-srv", Version: "v0.1.0",
	}, nil)
	ping, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "ping",
			Description: "responds with pong",
			InputSchema: `{"type":"object"}`,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, args string) (string, error) {
			return "pong", nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}
	err = mcp.RegisterTools(mcpServer, ping)
	if err != nil {
		t.Fatalf("register tools: %v", err)
	}

	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(mcpServer, mcp.HTTPServerOptions{}))
	defer httpServer.Close()

	// 2. Construct the engine pointing at the http MCP endpoint.
	stub := newStubModel("ping", `{}`, "pong-received")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{{Name: "test", Transport: mcp.TransportHTTP, Endpoint: httpServer.URL}},
	})
	defer eng.Close()

	// 3. The remote tool must appear in the merged list under its
	// prefixed name (DefaultNaming → "<source>_<tool>").
	want := "test_ping"
	found := false
	for _, tool := range eng.Tools() {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(eng.Tools()))
		for _, t := range eng.Tools() {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in engine.Tools(); got %v", want, names)
	}
}

// TestEngine_DialMCPServer_RejectsDuplicateNames ensures the
// fail-fast guard fires on misconfiguration: two MCPServer
// entries with the same Name must abort engine.New rather than
// silently overwriting.
func TestEngine_DialMCPServer_RejectsDuplicateNames(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{
			{Name: "dup", Transport: mcp.TransportHTTP, Endpoint: "http://example.invalid/"},
			{Name: "dup", Transport: mcp.TransportHTTP, Endpoint: "http://other.invalid/"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

// TestEngine_DialMCPServer_RejectsBadEndpoint surfaces dial
// failures at engine.New time so operators don't discover the
// problem on the first tool call.
func TestEngine_DialMCPServer_RejectsBadEndpoint(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{
			{Name: "bad", Transport: mcp.TransportHTTP, Endpoint: ""}, // empty endpoint fails Validate
		},
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestEngine_DialMCPServer_Stdio re-execs this test binary as a
// stdio MCP server (see TestMain) and verifies engine.New dials it
// over stdin/stdout, lists its `ping` tool, and surfaces it under
// the `stdio_ping` prefix. Close must terminate the subprocess
// cleanly.
//
// Skipped when the test binary path cannot be resolved (uncommon —
// `go test` always provides argv[0]).
func TestEngine_DialMCPServer_Stdio(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable not available: %v", err)
	}
	_, err = exec.LookPath(self)
	if err != nil && !fileExists(self) {
		t.Skipf("test binary unreachable for re-exec: %v", err)
	}

	stub := newStubModel("ping", `{}`, "")
	client, _ := chat.NewClient(stub)

	eng := mustEngineWith(t, client, toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{{
			Name:      "stdio",
			Transport: mcp.TransportStdio,
			Command:   self,
			Args:      []string{"-test.run=^$"}, // no test selector — TestMain re-routes
			Env:       append(os.Environ(), runAsMCPServerEnv+"=1"),
		}},
	})
	defer eng.Close()

	want := "stdio_ping"
	found := false
	for _, tool := range eng.Tools() {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(eng.Tools()))
		for _, t := range eng.Tools() {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in engine.Tools(); got %v", want, names)
	}
}

// TestEngine_DialMCPServer_StdioRejectsEmptyCommand mirrors the
// HTTP empty-endpoint guard for the stdio path.
func TestEngine_DialMCPServer_StdioRejectsEmptyCommand(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{{
			Name:      "bad",
			Transport: mcp.TransportStdio,
		}},
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// fileExists is a tiny helper used only by the stdio test's
// re-exec sanity check.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestEngine_DialMCPServers_ToleratesUnreachable verifies boot tolerance
// (B3b-1): a well-formed but unreachable server is recorded "failed" with its
// reason and skipped, so engine.New still succeeds and serves the rest —
// replacing the old all-or-nothing boot. (A malformed config stays fatal, as
// the sibling Rejects* tests assert.)
func TestEngine_DialMCPServers_ToleratesUnreachable(t *testing.T) {
	stub := newStubModel("nop", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{
			{Name: "down", Transport: mcp.TransportHTTP, Endpoint: "http://127.0.0.1:1/mcp"},
		},
	})
	t.Cleanup(func() { _ = eng.Close() })

	statuses := eng.MCPServerStatuses()
	if len(statuses) != 1 || statuses[0].Name != "down" || statuses[0].Status != "failed" || statuses[0].Err == nil {
		t.Fatalf("statuses = %+v, want [down failed <reason>]", statuses)
	}
	tools, err := eng.MCPTools(context.Background(), "")
	if err != nil {
		t.Fatalf("MCPTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty (no connected server)", tools)
	}
}

// TestEngine_ReconnectMCPServer covers the reconnect path (B3b-2) against an
// unreachable server: the dial still fails, so the server walks connecting →
// failed (returning the error) and its tools stay absent; an unknown name is
// ErrUnknownMCPServer. (A successful reconnect's tool hot-swap rides the same
// code path as boot, which the stdio integration test already exercises.)
func TestEngine_ReconnectMCPServer(t *testing.T) {
	stub := newStubModel("nop", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		MCPServers: []mcp.ServerConfig{
			{Name: "down", Transport: mcp.TransportHTTP, Endpoint: "http://127.0.0.1:1/mcp"},
		},
	})
	t.Cleanup(func() { _ = eng.Close() })

	if err := eng.ReconnectMCPServer(context.Background(), "down"); err == nil {
		t.Fatal("reconnect of an unreachable server must return the dial error")
	}
	st := eng.MCPServerStatuses()
	if len(st) != 1 || st[0].Status != "failed" || st[0].Err == nil {
		t.Fatalf("statuses = %+v, want [down failed <reason>]", st)
	}
	if tools, _ := eng.MCPTools(context.Background(), ""); len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty after a failed reconnect", tools)
	}

	if err := eng.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, ErrUnknownMCPServer) {
		t.Fatalf("reconnect unknown = %v, want ErrUnknownMCPServer", err)
	}
}
