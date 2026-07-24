package toolset_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestDiagnosticRegistryListsOnlyDirectTools(t *testing.T) {
	registry := toolset.NewDiagnosticRegistry()

	found, err := registry.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantClasses := map[string]tool.SafetyClass{
		"read": tool.SafetyClassSafe,
		"glob": tool.SafetyClassSafe,
		"grep": tool.SafetyClassSafe,
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
	if len(got) != len(wantClasses) {
		t.Fatalf("direct tool count = %d, want %d (%v)", len(got), len(wantClasses), got)
	}
}

func TestDiagnosticRegistryInvokesWithinRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("lyra"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := toolset.NewDiagnosticRegistry()
	output, err := registry.Invoke(t.Context(), root, "read", `{"file_path":"note.txt"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if value := output.Any(); !strings.Contains(value.(map[string]any)["content"].(string), "lyra") {
		t.Errorf("Invoke output missing lyra: %#v", value)
	}
}

func TestDiagnosticRegistryRejectsUnknownOrEscapingTool(t *testing.T) {
	registry := toolset.NewDiagnosticRegistry()
	if _, err := registry.Invoke(t.Context(), t.TempDir(), "shell", "{}"); err == nil {
		t.Fatal("Invoke error = nil, want unknown-tool error")
	}
	outside := t.TempDir()
	if _, err := registry.Invoke(t.Context(), outside, "read", `{"file_path":"../escape"}`); err == nil {
		t.Fatal("Invoke escaping path error = nil")
	}
	if _, err := registry.Invoke(t.Context(), outside, "glob", `{"pattern":"../**/*"}`); err == nil {
		t.Fatal("Invoke escaping glob pattern error = nil")
	}
}

func TestDiagnosticRegistryRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not in workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}

	_, err := toolset.NewDiagnosticRegistry().Invoke(t.Context(), root, "read", `{"file_path":"outside/secret.txt"}`)
	if !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("Invoke symlink escape error = %v, want ErrPathOutsideRoot", err)
	}
}
