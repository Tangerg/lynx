package toolset_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/tools"
)

func TestRegistryListsCatalogMetadata(t *testing.T) {
	registry := buildRegistry(t)

	found, err := registry.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantClasses := map[string]tool.SafetyClass{
		"read":        tool.SafetyClassSafe,
		"glob":        tool.SafetyClassSafe,
		"grep":        tool.SafetyClassSafe,
		"write":       tool.SafetyClassWrite,
		"edit":        tool.SafetyClassWrite,
		"apply_patch": tool.SafetyClassWrite,
		"shell":       tool.SafetyClassExec,
		"task":        tool.SafetyClassSafe,
	}
	got := make(map[string]tool.SafetyClass, len(found))
	for _, candidate := range found {
		got[candidate.Name] = candidate.SafetyClass
		if candidate.Schema.Map() == nil {
			t.Errorf("tool %q has nil schema object", candidate.Name)
		}
		if candidate.Description == "" {
			t.Errorf("tool %q has empty description", candidate.Name)
		}
	}
	for name, want := range wantClasses {
		if got[name] != want {
			t.Errorf("tool %q safety = %q, want %q", name, got[name], want)
		}
	}
}

func TestRegistryInvokesCatalogTool(t *testing.T) {
	registry := buildRegistry(t)
	output, err := registry.Invoke(t.Context(), "shell", `{"command":"echo lyra"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(output, "lyra") {
		t.Errorf("Invoke output missing lyra: %q", output)
	}
}

func TestRegistryRejectsUnknownTool(t *testing.T) {
	registry := buildRegistry(t)
	if _, err := registry.Invoke(t.Context(), "no-such-tool", "{}"); err == nil {
		t.Fatal("Invoke error = nil, want unknown-tool error")
	}
}

func buildRegistry(t *testing.T) tool.Registry {
	t.Helper()
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{})
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
	task, err := tools.New[struct{}, string](tools.Config{
		Name:        "task",
		Description: "Delegate one task.",
	}, func(context.Context, struct{}) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatalf("task tool: %v", err)
	}
	built.Resolver.UseTaskTool(task)
	registry, err := toolset.NewRegistry(built.Resolver)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}
