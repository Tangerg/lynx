package agentexec

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type projectionTool struct {
	name     string
	deferred []string // when non-nil, this tool declares withheld names
}

func (t projectionTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (projectionTool) Call(context.Context, string) (string, error) { return "", nil }

func (t projectionTool) DeferredToolNames() []string { return t.deferred }

func advertisedNames(t *testing.T, actionTools []tools.Tool) map[string]bool {
	t.Helper()
	registry, err := tools.NewRegistry(actionTools...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defs := advertisedTools(actionTools, registry)
	out := make(map[string]bool, len(defs))
	for _, d := range defs {
		out[d.Name] = true
	}
	return out
}

func TestAdvertisedToolsWithholdsDeferredNames(t *testing.T) {
	// search declares the two MCP tools deferred; they stay registered (resolvable)
	// but must not appear in the advertised manifest.
	search := projectionTool{name: "search_tools", deferred: []string{"mcp_a", "mcp_b"}}
	actionTools := []tools.Tool{
		projectionTool{name: "read"},
		projectionTool{name: "mcp_a"},
		projectionTool{name: "mcp_b"},
		search,
	}
	advertised := advertisedNames(t, actionTools)

	if !advertised["read"] || !advertised["search_tools"] {
		t.Fatalf("first-party tools must stay advertised: %v", advertised)
	}
	if advertised["mcp_a"] || advertised["mcp_b"] {
		t.Fatalf("deferred MCP tools must be withheld from the manifest: %v", advertised)
	}
}

func TestAdvertisedToolsUnchangedWithoutDeferral(t *testing.T) {
	actionTools := []tools.Tool{
		projectionTool{name: "read"},
		projectionTool{name: "write"},
	}
	advertised := advertisedNames(t, actionTools)
	if !advertised["read"] || !advertised["write"] || len(advertised) != 2 {
		t.Fatalf("no deferral should advertise every tool: %v", advertised)
	}
}
