package engine

import (
	"context"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/mcp"
)

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

	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(mcpServer, nil))
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
