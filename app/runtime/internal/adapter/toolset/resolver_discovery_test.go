package toolset

import (
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// resolveRootManifest builds one exact root visibility snapshot.
func resolveRootManifest(t *testing.T, mcpTools []toolcontract.Tool) Manifest {
	t.Helper()
	built, err := Build(t.Context(), BuildConfig{DefaultCWD: t.TempDir(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	built.Resolver.SetMCPTools(mcpTools)

	manifest, err := built.Resolver.Manifest(t.Context(), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	return manifest
}

type deferredNamer interface{ DeferredToolNames() []string }

func TestResolverOffersSearchToolsOverDeferredCatalog(t *testing.T) {
	mcpTools := []toolcontract.Tool{
		mcpToolStub{name: "files_read", server: "files", remote: "read"},
		mcpToolStub{name: "files_write", server: "files", remote: "write"},
	}
	resolved := manifestTools(resolveRootManifest(t, mcpTools))

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
	for _, want := range []string{"files_read", "files_write", "lsp"} {
		if !deferred[want] {
			t.Errorf("deferred names = %v, missing %q", deferred, want)
		}
	}
}

func TestResolverDefersRuntimeToolsWithoutMCP(t *testing.T) {
	manifest := resolveRootManifest(t, nil)
	advertised := definitionNames(manifest.Visible)
	for _, direct := range []string{catalog.Read, catalog.Glob, catalog.Grep, catalog.ApplyPatch, catalog.Shell, catalog.SearchTools} {
		if !advertised[direct] {
			t.Errorf("initial manifest = %v, missing direct tool %q", advertised, direct)
		}
	}
	for _, deferred := range []string{"lsp"} {
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
