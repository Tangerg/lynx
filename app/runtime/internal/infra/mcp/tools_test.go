package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	lynxmcp "github.com/Tangerg/lynx/mcp"
	toolcontract "github.com/Tangerg/lynx/tool"
)

type concurrencyKeyer interface {
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}

func TestInputSchemaRejectsMissingAndInvalidValues(t *testing.T) {
	if _, err := inputSchema(nil); !errors.Is(err, mcpserver.ErrInvalidInputSchema) {
		t.Fatalf("inputSchema(nil) error = %v, want ErrInvalidInputSchema", err)
	}
	if _, err := inputSchema(map[string]any{"type": "array"}); !errors.Is(err, mcpserver.ErrInvalidInputSchema) {
		t.Fatalf("inputSchema(array) error = %v, want ErrInvalidInputSchema", err)
	}
	if _, err := inputSchema(make(chan int)); !errors.Is(err, mcpserver.ErrInvalidInputSchema) {
		t.Fatalf("inputSchema(channel) error = %v, want ErrInvalidInputSchema", err)
	}
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

func TestRemoteToolCatalogRejectsUnboundedMaterial(t *testing.T) {
	t.Run("model-facing description", func(t *testing.T) {
		session := toolCatalogSession(t, &sdkmcp.Tool{
			Name:        "oversized-description",
			Description: strings.Repeat("x", mcpserver.MaxRemoteToolDescriptionBytes+1),
			InputSchema: json.RawMessage(`{"type":"object"}`),
		})
		if _, err := sourceTools(t.Context(), lynxmcp.ToolSource{Name: "catalog", Session: session}); err == nil {
			t.Fatal("sourceTools accepted a description larger than 64 KiB")
		}
	})

	t.Run("management schema", func(t *testing.T) {
		session := toolCatalogSession(t, &sdkmcp.Tool{
			Name: "oversized-schema",
			InputSchema: json.RawMessage(`{"type":"object","description":"` +
				strings.Repeat("x", mcpserver.MaxRemoteToolInputSchemaBytes+1) + `"}`),
		})
		connections := &Connections{servers: []*server{{
			config:  ServerConfig{Name: "catalog"},
			session: session,
		}}}
		if _, err := connections.Tools(t.Context(), "catalog"); err == nil {
			t.Fatal("Connections.Tools accepted a schema larger than 1 MiB")
		}
	})

	t.Run("tool count", func(t *testing.T) {
		candidate := make([]toolcontract.Tool, mcpserver.MaxRemoteToolsPerServer+1)
		for index := range candidate {
			candidate[index] = catalogTool(fmt.Sprintf("catalog_tool_%04d", index))
		}
		if err := validateToolCatalog(nil, nil, "catalog", candidate); err == nil {
			t.Fatal("validateToolCatalog accepted more than 2,048 remote tools")
		}
	})
}

func toolCatalogSession(t *testing.T, descriptors ...*sdkmcp.Tool) *sdkmcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v0.1.0"}, nil)
	for _, descriptor := range descriptors {
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
	return clientSession
}
