package toolset_test

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/agent/core"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// runAsMCPServerEnv is the env-var sentinel that flips this test
// binary into "act as a stdio MCP server" mode. The stdio integration
// test (below) re-execs the test binary with this set so we get a
// real subprocess to talk to over stdin/stdout without depending on
// `npx` / Python / etc. in CI.
const runAsMCPServerEnv = "LYRA_TEST_RUN_AS_MCP_SERVER"

func resolvedCodingTools(t *testing.T, resolver *toolset.Resolver) []tools.Tool {
	t.Helper()
	group, ok, err := resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleCoding})
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	values, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("coding tools: %v", err)
	}
	return values
}

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
	ping, err := tools.New[struct{}, string](
		tools.Config{
			Name:        "ping",
			Description: "responds with pong",
		},
		func(context.Context, struct{}) (string, error) { return "pong", nil },
	)
	if err != nil {
		log.Fatalf("build tool: %v", err)
	}
	if err := lynxmcp.Register(srv, ping); err != nil {
		log.Fatalf("register tools: %v", err)
	}
	transport := &sdkmcp.StdioTransport{}
	if err := srv.Run(context.Background(), transport); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// TestToolEnvironmentDialsMCPServer brings up an in-process MCP server
// over HTTP, registers one tool against it, then constructs an
// tool environment wired to dial the server. Its catalog must
// include the remote tool under its prefixed name; cleanup must
// drop the session cleanly.
func TestToolEnvironmentDialsMCPServer(t *testing.T) {
	// 1. Spin up a real MCP server with one tool.
	mcpServer := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name: "test-srv", Version: "v0.1.0",
	}, nil)
	ping, err := tools.New[struct{}, string](
		tools.Config{
			Name:        "ping",
			Description: "responds with pong",
		},
		func(context.Context, struct{}) (string, error) {
			return "pong", nil
		},
	)
	if err != nil {
		t.Fatalf("build tool: %v", err)
	}
	err = lynxmcp.Register(mcpServer, ping)
	if err != nil {
		t.Fatalf("register tools: %v", err)
	}

	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return mcpServer },
		nil,
	))
	t.Cleanup(httpServer.Close)

	// 2. Construct the tool environment pointing at the HTTP MCP endpoint.
	built := mustToolEnvironment(t, toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{{Name: "test", Transport: mcpserver.TransportStreamableHTTP, Endpoint: httpServer.URL}},
	})

	// 3. The remote tool must appear in the merged list under its
	// model-facing MCP port name.
	want := "test_ping"
	found := false
	for _, tool := range resolvedCodingTools(t, built.Resolver) {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		catalog := resolvedCodingTools(t, built.Resolver)
		names := make([]string, 0, len(catalog))
		for _, t := range catalog {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in tool catalog; got %v", want, names)
	}
}

// TestToolEnvironmentRejectsDuplicateMCPNames ensures the
// fail-fast guard fires on misconfiguration: two MCPServer
// entries with the same Name must abort tool construction rather than
// silently overwriting.
func TestToolEnvironmentRejectsDuplicateMCPNames(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{
			{Name: "dup", Transport: mcpserver.TransportStreamableHTTP, Endpoint: "http://example.invalid/"},
			{Name: "dup", Transport: mcpserver.TransportStreamableHTTP, Endpoint: "http://other.invalid/"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

// TestToolEnvironmentRejectsBadMCPEndpoint surfaces validation
// failures at build time so operators don't discover the
// problem on the first tool call.
func TestToolEnvironmentRejectsBadMCPEndpoint(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{
			{Name: "bad", Transport: mcpserver.TransportStreamableHTTP, Endpoint: ""}, // empty endpoint fails Validate
		},
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

// TestToolEnvironmentDialsStdioMCP re-execs this test binary as a
// stdio MCP server (see TestMain) and verifies tool construction dials it
// over stdin/stdout, lists its `ping` tool, and surfaces it under
// the `stdio_ping` prefix. Close must terminate the subprocess
// cleanly.
//
// Skipped when the test binary path cannot be resolved (uncommon —
// `go test` always provides argv[0]).
func TestToolEnvironmentDialsStdioMCP(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable not available: %v", err)
	}
	_, err = exec.LookPath(self)
	if err != nil && !fileExists(self) {
		t.Skipf("test binary unreachable for re-exec: %v", err)
	}

	built := mustToolEnvironment(t, toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{{
			Name:      "stdio",
			Transport: mcpserver.TransportStdio,
			Command:   self,
			Args:      []string{"-test.run=^$"}, // no test selector — TestMain re-routes
			Env:       append(os.Environ(), runAsMCPServerEnv+"=1"),
		}},
	})
	want := "stdio_ping"
	found := false
	for _, tool := range resolvedCodingTools(t, built.Resolver) {
		if tool.Definition().Name == want {
			found = true
			break
		}
	}
	if !found {
		catalog := resolvedCodingTools(t, built.Resolver)
		names := make([]string, 0, len(catalog))
		for _, t := range catalog {
			names = append(names, t.Definition().Name)
		}
		t.Fatalf("tool %q not in tool catalog; got %v", want, names)
	}
}

// TestToolEnvironmentRejectsEmptyStdioCommand mirrors the
// HTTP empty-endpoint guard for the stdio path.
func TestToolEnvironmentRejectsEmptyStdioCommand(t *testing.T) {
	_, err := toolset.Build(context.Background(), toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{{
			Name:      "bad",
			Transport: mcpserver.TransportStdio,
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

// TestToolEnvironmentToleratesUnreachableMCP verifies boot tolerance
// (B3b-1): a well-formed but unreachable server is recorded "failed" with its
// reason and skipped, so tool construction still succeeds and serves the rest —
// replacing the old all-or-nothing boot. (A malformed config stays fatal, as
// the sibling Rejects* tests assert.)
func TestToolEnvironmentToleratesUnreachableMCP(t *testing.T) {
	built := mustToolEnvironment(t, toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{
			{Name: "down", Transport: mcpserver.TransportStreamableHTTP, Endpoint: "http://127.0.0.1:1/mcp"},
		},
	})
	statuses := built.MCPStatusReader.Statuses()
	if len(statuses) != 1 || statuses[0].Name != "down" || statuses[0].State != mcpserver.ConnectionFailed || statuses[0].Err == nil {
		t.Fatalf("statuses = %+v, want [down failed <reason>]", statuses)
	}
	tools, err := built.MCPToolCatalog.Tools(context.Background(), "")
	if err != nil {
		t.Fatalf("MCPTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty (no connected server)", tools)
	}
}

// TestToolEnvironmentReconnectsMCP covers the reconnect path against an
// unreachable server: the dial still fails, so the server walks connecting →
// failed (returning the error) and its tools stay absent; an unknown name is
// mcpserver.ErrUnknownServer. (A successful reconnect's tool hot-swap rides the same
// code path as boot, which the stdio integration test already exercises.)
func TestToolEnvironmentReconnectsMCP(t *testing.T) {
	built := mustToolEnvironment(t, toolset.BuildConfig{
		MCPServers: []mcpserver.LiveConfig{
			{Name: "down", Transport: mcpserver.TransportStreamableHTTP, Endpoint: "http://127.0.0.1:1/mcp"},
		},
	})
	if err := built.MCPConnectionCommands.Reconnect(context.Background(), "down"); err == nil {
		t.Fatal("reconnect of an unreachable server must return the dial error")
	}
	st := built.MCPStatusReader.Statuses()
	if len(st) != 1 || st[0].State != mcpserver.ConnectionFailed || st[0].Err == nil {
		t.Fatalf("statuses = %+v, want [down failed <reason>]", st)
	}
	if tools, _ := built.MCPToolCatalog.Tools(context.Background(), ""); len(tools) != 0 {
		t.Fatalf("MCPTools = %+v, want empty after a failed reconnect", tools)
	}

	if err := built.MCPConnectionCommands.Reconnect(context.Background(), "ghost"); !errors.Is(err, mcpserver.ErrUnknownServer) {
		t.Fatalf("reconnect unknown = %v, want mcpserver.ErrUnknownServer", err)
	}
}

func mustToolEnvironment(t *testing.T, config toolset.BuildConfig) toolset.Built {
	t.Helper()
	built, err := toolset.Build(t.Context(), config)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() {
		for index := len(built.Closers) - 1; index >= 0; index-- {
			if closeFn := built.Closers[index]; closeFn != nil {
				_ = closeFn()
			}
		}
	})
	return built
}
