package toolset

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/tools"
)

// resolveCodingTools builds a resolver, injects mcpTools, and returns the coding
// role's fully resolved tool set (MCP tools still resolvable; deferral is a
// manifest-projection concern applied later in the turn, not here).
func resolveCodingTools(t *testing.T, mcpTools []tools.Tool) []tools.Tool {
	t.Helper()
	built, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools(mcpTools)

	group, ok, err := built.Resolver.Resolve(t.Context(), core.ToolGroupRequirement{Role: toolport.ToolRoleCoding})
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	resolved, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	return resolved
}

type deferredNamer interface{ DeferredToolNames() []string }

func TestResolverOffersSearchToolsOverMCPCatalog(t *testing.T) {
	mcpTools := []tools.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	}
	resolved := resolveCodingTools(t, mcpTools)

	var search deferredNamer
	names := make(map[string]bool, len(resolved))
	for _, tool := range resolved {
		names[tool.Definition().Name] = true
		if d, ok := tool.(deferredNamer); ok {
			search = d
		}
	}

	// The MCP tools stay resolvable (in the set) AND a search_tools tool is added.
	if !names["files_read"] || !names["files_write"] {
		t.Fatalf("MCP tools must remain resolvable: %v", names)
	}
	if !names["search_tools"] {
		t.Fatalf("search_tools must be offered when MCP tools exist: %v", names)
	}
	if search == nil {
		t.Fatal("no tool reports deferred names")
	}
	deferred := search.DeferredToolNames()
	if len(deferred) != 2 {
		t.Fatalf("deferred names = %v, want the two MCP tools", deferred)
	}
}

func TestResolverOmitsSearchToolsWithoutMCP(t *testing.T) {
	resolved := resolveCodingTools(t, nil)
	for _, tool := range resolved {
		if tool.Definition().Name == "search_tools" {
			t.Fatal("search_tools must be absent when nothing is deferred")
		}
	}
}
