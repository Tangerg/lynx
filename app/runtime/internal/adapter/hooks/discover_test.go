package hooks

import (
	"context"
	"errors"
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

	hooks, err := Load(context.Background(), cwd, home)
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

func TestLoad_MalformedReturnsError(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, cwd, `{ this is not json `)

	if _, err := Load(context.Background(), cwd, home); err == nil {
		t.Fatal("Load malformed config error = nil")
	}
}

func TestLoadRejectsInvalidHookConfiguration(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown event", body: `{"hooks":[{"event":"PreTool","command":"check"}]}`},
		{name: "missing action", body: `{"hooks":[{"event":"Stop"}]}`},
		{name: "ambiguous action", body: `{"hooks":[{"event":"Stop","command":"notify","inject":"context"}]}`},
		{name: "negative timeout", body: `{"hooks":[{"event":"Stop","command":"notify","timeoutMs":-1}]}`},
		{name: "timeout on inject", body: `{"hooks":[{"event":"SessionStart","inject":"context","timeoutMs":100}]}`},
		{name: "matcher on non-tool event", body: `{"hooks":[{"event":"Stop","command":"notify","matcher":"shell"}]}`},
		{name: "malformed matcher", body: `{"hooks":[{"event":"PreToolUse","command":"check","matcher":"["}]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cwd := t.TempDir()
			writeHooks(t, cwd, test.body)
			if _, err := Load(t.Context(), cwd, ""); !errors.Is(err, domainhooks.ErrInvalidHook) {
				t.Fatalf("Load error = %v, want ErrInvalidHook", err)
			}
		})
	}
}

func TestLoad_MissingFilesAreFine(t *testing.T) {
	hooks, err := Load(context.Background(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 0 {
		t.Errorf("no files -> no hooks, got %+v", hooks)
	}
}

func TestLoadPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Load(ctx, t.TempDir(), t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled", err)
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
