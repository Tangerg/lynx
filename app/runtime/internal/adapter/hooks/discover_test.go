package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func writeHooks(t *testing.T, dir, body string) {
	t.Helper()
	lyra := filepath.Join(dir, ".lyra")
	if err := os.MkdirAll(lyra, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lyra, "hooks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_TagsGlobalAndProjectScope(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeHooks(t, home, `{"hooks":[{"event":"SessionStart","inject":"global-ctx"}]}`)
	writeHooks(t, cwd, `{"hooks":[{"event":"PreToolUse","matcher":"shell","command":"true"}]}`)

	hooks, err := Load(context.Background(), cwd, home, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(hooks), hooks)
	}
	if hooks[0].Scope != domainhooks.ScopeGlobal || hooks[0].Inject != "global-ctx" {
		t.Errorf("hook[0] = %+v, want global SessionStart", hooks[0])
	}
	if hooks[1].Scope != domainhooks.ScopeProject || hooks[1].Event != domainhooks.PreToolUse {
		t.Errorf("hook[1] = %+v, want project PreToolUse", hooks[1])
	}
	if hooks[1].Source == "" {
		t.Error("project hook missing Source provenance")
	}
}

func TestLoad_MalformedReportedAndSkipped(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, cwd, `{ this is not json `)

	var bad string
	hooks, err := Load(context.Background(), cwd, home, func(path string, _ error) { bad = path })
	if err != nil {
		t.Fatalf("Load must not fail on a malformed file: %v", err)
	}
	if len(hooks) != 0 {
		t.Errorf("malformed file should yield no hooks, got %+v", hooks)
	}
	if bad == "" {
		t.Error("onParseError not called for the malformed file")
	}
}

func TestLoad_MissingFilesAreFine(t *testing.T) {
	hooks, err := Load(context.Background(), t.TempDir(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 0 {
		t.Errorf("no files -> no hooks, got %+v", hooks)
	}
}

func TestProjectRoot_FindsGitAncestor(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(sub); got != root {
		t.Errorf("ProjectRoot(%q) = %q, want %q", sub, got, root)
	}
}
