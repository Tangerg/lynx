package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

type concurrencyKeyer interface {
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}

func TestSourceToolsEnablesOnlyAnnotatedReadOnlyConcurrency(t *testing.T) {
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	for _, descriptor := range []*sdkmcp.Tool{
		{
			Name:        "lookup",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Annotations: &sdkmcp.ToolAnnotations{ReadOnlyHint: true},
		},
		{
			Name:        "mutate",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	} {
		server.AddTool(descriptor, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return &sdkmcp.CallToolResult{}, nil
		})
	}
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.1.0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	wrapped, err := sourceTools(t.Context(), lynxmcp.ToolSource{Name: "catalog", Session: clientSession})
	if err != nil {
		t.Fatalf("sourceTools: %v", err)
	}
	if len(wrapped) != 2 {
		t.Fatalf("sourceTools count = %d, want 2", len(wrapped))
	}

	got := make(map[string]bool, len(wrapped))
	for _, tool := range wrapped {
		keyer, ok := tool.(concurrencyKeyer)
		if !ok {
			t.Fatalf("tool %q does not expose concurrency policy", tool.Definition().Name)
		}
		key, concurrent := keyer.ConcurrencyKey(`{"id":"one"}`)
		if key != "" {
			t.Fatalf("tool %q concurrency key = %q, want empty", tool.Definition().Name, key)
		}
		got[tool.Definition().Name] = concurrent
	}
	if !got["catalog_lookup"] || got["catalog_mutate"] {
		t.Fatalf("source tool concurrency = %v, want lookup=true mutate=false", got)
	}
}
