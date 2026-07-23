package toolset

import (
	"context"
	"slices"
	"testing"

	toolapp "github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools"
)

func TestRegistryTracksMCPToolHotSwap(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	first := catalogTestTool(t, "mcp_first", "first")
	second := catalogTestTool(t, "mcp_second", "second")
	registry, err := NewRegistry(built.Resolver)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	built.Resolver.SetMCPTools([]tools.Tool{first})
	if !hasCatalogTool(t, registry, "mcp_first") {
		t.Fatal("tools.list did not expose the current MCP tool")
	}
	if output, err := registry.Invoke(t.Context(), "mcp_first", `{}`); err != nil || output != "first" {
		t.Fatalf("tools.invoke(first) = (%q, %v), want (first, nil)", output, err)
	}

	built.Resolver.SetMCPTools([]tools.Tool{second})
	if hasCatalogTool(t, registry, "mcp_first") || !hasCatalogTool(t, registry, "mcp_second") {
		t.Fatal("tools.list retained a disconnected MCP catalog snapshot")
	}
	if _, err := registry.Invoke(t.Context(), "mcp_first", `{}`); err == nil {
		t.Fatal("tools.invoke resolved a disconnected MCP tool")
	}
	if output, err := registry.Invoke(t.Context(), "mcp_second", `{}`); err != nil || output != "second" {
		t.Fatalf("tools.invoke(second) = (%q, %v), want (second, nil)", output, err)
	}
}

func catalogTestTool(t *testing.T, name, output string) tools.Tool {
	t.Helper()
	tool, err := tools.New[struct{}, string](tools.Config{Name: name, Description: name}, func(_ context.Context, _ struct{}) (string, error) {
		return output, nil
	})
	if err != nil {
		t.Fatalf("new tool %q: %v", name, err)
	}
	return tool
}

func hasCatalogTool(t *testing.T, registry toolapp.Registry, name string) bool {
	t.Helper()
	catalog, err := registry.List(t.Context())
	if err != nil {
		t.Fatalf("tools.list: %v", err)
	}
	return slices.ContainsFunc(catalog, func(candidate tool.Tool) bool { return candidate.Name == name })
}
