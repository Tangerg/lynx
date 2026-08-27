package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/core/chat"
)

type patchPathStub struct {
	called *bool
}

func (p *patchPathStub) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "apply_patch", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (p *patchPathStub) Call(context.Context, string) (string, error) {
	*p.called = true
	return "patched", nil
}

func (p *patchPathStub) MutationPaths(string) ([]string, error) {
	return []string{"ok.txt", ".git/config"}, nil
}

func TestPathGuardApplyPatchChecksAllTargets(t *testing.T) {
	called := false
	tool := withPathGuard(&patchPathStub{called: &called}, "/work")
	patch := `--- a/ok.txt
+++ b/ok.txt
@@ -1 +1 @@
-old
+new
--- a/.git/config
+++ b/.git/config
@@ -1 +1 @@
-old
+new
`
	arguments, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Call(t.Context(), string(arguments))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if called {
		t.Fatal("inner tool ran despite protected path in patch")
	}
	if !strings.Contains(out, "Refused") {
		t.Fatalf("out = %q, want refusal", out)
	}
}

type pathGuardArgs struct {
	Path string `json:"path"`
}

type failingMutationReporter struct {
	toolcontract.Tool
	err error
}

func (f failingMutationReporter) MutationPaths(string) ([]string, error) {
	return nil, f.err
}

func TestWithPathGuardFailsClosedWhenMutationDiscoveryFails(t *testing.T) {
	cause := errors.New("mutation discovery failed")
	called := false
	inner, _ := toolcontract.NewFunc[pathGuardArgs, string](
		toolcontract.FuncConfig{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)

	_, err := withPathGuard(failingMutationReporter{Tool: inner, err: cause}, t.TempDir()).
		Call(t.Context(), `{"path":"safe.txt"}`)
	if !errors.Is(err, cause) {
		t.Fatalf("Call error = %v, want mutation-discovery cause", err)
	}
	if called {
		t.Fatal("path guard executed the tool without authoritative mutation paths")
	}
}

// TestWithPathGuard verifies the VCS-metadata write barrier: writes whose
// resolved path lands inside a .git directory (directly, nested, or via a
// "../" traversal) are refused without running the inner tool, while
// ordinary paths — including non-.git dotfiles — pass through untouched.
func TestWithPathGuard(t *testing.T) {
	called := false
	inner, _ := toolcontract.NewFunc[pathGuardArgs, string](
		toolcontract.FuncConfig{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)
	guarded := withPathGuard(inner, "/work")

	cases := []struct {
		name      string
		path      string
		wantBlock bool
	}{
		{"git hook", ".git/hooks/pre-commit", true},
		{"git config", ".git/config", true},
		{"nested repo git", "vendor/lib/.git/config", true},
		{"traversal into git", "../work/.git/x", true},
		{"ordinary file", "internal/main.go", false},
		{"non-git dotfile", ".env", false},
		{"dir merely containing git in name", "gitignore/notes.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			out, err := guarded.Call(context.Background(), `{"path":"`+tc.path+`"}`)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantBlock {
				if called {
					t.Fatalf("inner tool ran for protected path %q", tc.path)
				}
				if !strings.Contains(out, "Refused") {
					t.Fatalf("path %q not refused: %q", tc.path, out)
				}
				return
			}
			if !called {
				t.Fatalf("inner tool was blocked for allowed path %q: %q", tc.path, out)
			}
			if out != "wrote" {
				t.Fatalf("inner result lost for %q: %q", tc.path, out)
			}
		})
	}
}

// TestGuardMutationPathConfinesIsolatedWrites verifies the isolation boundary:
// in an isolated session a write must land inside the workspace copy, so an
// absolute path, a "../" escape, or a "~" path pointing at the real tree is
// refused — while the same paths are allowed for a non-isolated turn (the fs
// tools' normal absolute-path behavior).
func TestGuardMutationPathConfinesIsolatedWrites(t *testing.T) {
	work := t.TempDir()
	outside := filepath.Join(t.TempDir(), "real-project", "main.go")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		path      string
		wantBlock bool // when isolated
	}{
		{"relative inside", "pkg/main.go", false},
		{"workspace root file", "main.go", false},
		{"absolute outside", outside, true},
		{"absolute inside", filepath.Join(work, "in.go"), false},
		{"parent traversal", "../real-project/main.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Isolated: escapes are refused.
			refusal, ok := guardMutationPath(work, tc.path, true)
			if tc.wantBlock {
				if ok || !strings.Contains(refusal, "isolated session's sandbox") {
					t.Fatalf("isolated %q: ok=%v refusal=%q, want blocked at the sandbox boundary", tc.path, ok, refusal)
				}
			} else if !ok {
				t.Fatalf("isolated %q wrongly blocked: %q", tc.path, refusal)
			}
			// Non-isolated: the same path is always allowed (absolute is fine).
			if _, ok := guardMutationPath(work, tc.path, false); !ok {
				t.Fatalf("non-isolated %q was blocked but should be allowed", tc.path)
			}
		})
	}
}

func TestWithPathGuardRejectsSymlinkAliasesIntoGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(dir, "git-alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".git", "new-config"), filepath.Join(dir, "dangling-alias")); err != nil {
		t.Fatal(err)
	}

	called := false
	inner, _ := toolcontract.NewFunc[pathGuardArgs, string](
		toolcontract.FuncConfig{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)
	guarded := withPathGuard(inner, dir)
	for _, path := range []string{"git-alias/config", "dangling-alias"} {
		called = false
		out, err := guarded.Call(context.Background(), `{"path":"`+path+`"}`)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if called || !strings.Contains(out, "Refused") {
			t.Fatalf("symlink path %q escaped guard: called=%v out=%q", path, called, out)
		}
	}
}

func TestWithPathGuardRejectsSymlinkCycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("b", filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a", filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}

	called := false
	inner, _ := toolcontract.NewFunc[pathGuardArgs, string](
		toolcontract.FuncConfig{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)
	out, err := withPathGuard(inner, dir).Call(context.Background(), `{"path":"a/config"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(out, "Refused") {
		t.Fatalf("symlink cycle was not refused: called=%v out=%q", called, out)
	}
}
