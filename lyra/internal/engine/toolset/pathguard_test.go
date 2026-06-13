package toolset

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestWithPathGuard verifies the VCS-metadata write barrier: writes whose
// resolved path lands inside a .git directory (directly, nested, or via a
// "../" traversal) are refused without running the inner tool, while
// ordinary paths — including non-.git dotfiles — pass through untouched.
func TestWithPathGuard(t *testing.T) {
	called := false
	inner, _ := chat.NewTool(
		chat.ToolDefinition{Name: "write", Description: "stub", InputSchema: `{"type":"object"}`},
		chat.ToolMetadata{},
		func(context.Context, string) (string, error) {
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
