package toolset

import (
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	toolcontract "github.com/Tangerg/lynx/tool"

	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// resolveCodingTools builds a resolver, injects mcpTools, and returns the coding
// role's fully resolved tool set (MCP tools still resolvable; deferral is a
// manifest-projection concern applied later in the turn, not here).
func resolveCodingTools(t *testing.T, mcpTools []toolcontract.Tool) []toolcontract.Tool {
	t.Helper()
	built, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools(mcpTools)

	group, ok, err := built.Resolver.Resolve(t.Context(), domaintool.GroupCoding)
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

func TestResolverOffersSearchToolsOverDeferredCatalog(t *testing.T) {
	mcpTools := []toolcontract.Tool{
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
	deferred := nameSliceSet(search.DeferredToolNames())
	for _, want := range []string{"files_read", "files_write", "lsp", "lsp_diagnostics"} {
		if !deferred[want] {
			t.Errorf("deferred names = %v, missing %q", deferred, want)
		}
	}
}

func TestResolverDefersRuntimeToolsWithoutMCP(t *testing.T) {
	resolved := resolveCodingTools(t, nil)
	manifest, err := toolloop.Advertise(resolved)
	if err != nil {
		t.Fatalf("Advertise: %v", err)
	}
	advertised := make(map[string]bool, len(manifest))
	for _, definition := range manifest {
		advertised[definition.Name] = true
	}
	for _, direct := range []string{"read", "glob", "grep", "edit", "write", "shell", "search_tools"} {
		if !advertised[direct] {
			t.Errorf("initial manifest = %v, missing direct tool %q", advertised, direct)
		}
	}
	for _, deferred := range []string{"lsp", "lsp_diagnostics"} {
		if advertised[deferred] {
			t.Errorf("initial manifest = %v, unexpectedly advertised deferred tool %q", advertised, deferred)
		}
	}
}

func nameSliceSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
