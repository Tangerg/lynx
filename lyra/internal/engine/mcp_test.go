package engine

import (
	"context"
	"log"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
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
	if err := mcp.RegisterTools(mcpServer, ping); err != nil {
		t.Fatalf("register tools: %v", err)
	}

	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(mcpServer, mcp.HTTPServerOptions{}))
	defer httpServer.Close()

	// 2. Construct the engine pointing at the http MCP endpoint.
	stub := newStubModel("ping", `{}`, "pong-received")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{
		ChatClient: client,
		MCPServers: []MCPServer{{Name: "test", Endpoint: httpServer.URL}},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
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
	stub := newStubModel("noop", `{}`, "")
	client, _ := chat.NewClient(stub)

	_, err := New(Config{
		ChatClient: client,
		MCPServers: []MCPServer{
			{Name: "dup", Endpoint: "http://example.invalid/"},
			{Name: "dup", Endpoint: "http://other.invalid/"},
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
	stub := newStubModel("noop", `{}`, "")
	client, _ := chat.NewClient(stub)

	_, err := New(Config{
		ChatClient: client,
		MCPServers: []MCPServer{
			{Name: "bad", Endpoint: ""}, // empty endpoint fails validate()
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
	if _, err := exec.LookPath(self); err != nil && !fileExists(self) {
		t.Skipf("test binary unreachable for re-exec: %v", err)
	}

	stub := newStubModel("ping", `{}`, "")
	client, _ := chat.NewClient(stub)

	eng, err := New(Config{
		ChatClient: client,
		MCPServers: []MCPServer{{
			Name:      "stdio",
			Transport: MCPTransportStdio,
			Command:   self,
			Args:      []string{"-test.run=^$"}, // no test selector — TestMain re-routes
			Env:       append(os.Environ(), runAsMCPServerEnv+"=1"),
		}},
	})
	if err != nil {
		t.Fatalf("engine.New (stdio): %v", err)
	}
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
	stub := newStubModel("noop", `{}`, "")
	client, _ := chat.NewClient(stub)
	_, err := New(Config{
		ChatClient: client,
		MCPServers: []MCPServer{{
			Name:      "bad",
			Transport: MCPTransportStdio,
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
