package toolset

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/tools"
)

type pathGuardArgs struct {
	FilePath string `json:"file_path"`
}

// TestWithPathGuard verifies the VCS-metadata write barrier: writes whose
// resolved path lands inside a .git directory (directly, nested, or via a
// "../" traversal) are refused without running the inner tool, while
// ordinary paths — including non-.git dotfiles — pass through untouched.
func TestWithPathGuard(t *testing.T) {
	called := false
	inner, _ := tools.New[pathGuardArgs, string](
		tools.Config{Name: "write", Description: "stub"},
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
			out, err := guarded.Call(context.Background(), `{"file_path":"`+tc.path+`"}`)
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
	inner, _ := tools.New[pathGuardArgs, string](
		tools.Config{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)
	guarded := withPathGuard(inner, dir)
	for _, path := range []string{"git-alias/config", "dangling-alias"} {
		called = false
		out, err := guarded.Call(context.Background(), `{"file_path":"`+path+`"}`)
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
	inner, _ := tools.New[pathGuardArgs, string](
		tools.Config{Name: "write", Description: "stub"},
		func(context.Context, pathGuardArgs) (string, error) {
			called = true
			return "wrote", nil
		},
	)
	out, err := withPathGuard(inner, dir).Call(context.Background(), `{"file_path":"a/config"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(out, "Refused") {
		t.Fatalf("symlink cycle was not refused: called=%v out=%q", called, out)
	}
}
