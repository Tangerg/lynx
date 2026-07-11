package agentdoc_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
)

// TestRenderAnnotates confirms each file gets the `<!-- From: -->` provenance
// comment.
func TestRenderAnnotates(t *testing.T) {
	out := agentdoc.Render([]agentdoc.File{
		{Path: "/a/AGENTS.md", Content: "alpha"},
		{Path: "/b/AGENTS.md", Content: "beta"},
	}, agentdoc.DefaultMaxBytes)

	if !strings.Contains(out, "<!-- From: /a/AGENTS.md -->") {
		t.Fatalf("missing /a annotation: %q", out)
	}
	if !strings.Contains(out, "<!-- From: /b/AGENTS.md -->") {
		t.Fatalf("missing /b annotation: %q", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("missing content: %q", out)
	}
	if strings.Index(out, "alpha") > strings.Index(out, "beta") {
		t.Fatalf("alpha must precede beta in output")
	}
}

// TestRenderTrimsFromRoot confirms the budget trim discards earliest (root-most)
// files first since deeper = more specific = more valuable.
func TestRenderTrimsFromRoot(t *testing.T) {
	files := []agentdoc.File{
		{Path: "/root/AGENTS.md", Content: strings.Repeat("a", 1000)},
		{Path: "/leaf/AGENTS.md", Content: "leaf"},
	}
	out := agentdoc.Render(files, 200) // budget can fit only the second
	if strings.Contains(out, "/root/AGENTS.md") {
		t.Fatalf("root file should have been trimmed: %q", out)
	}
	if !strings.Contains(out, "leaf") {
		t.Fatalf("leaf file should survive: %q", out)
	}
}

// TestRenderReturnsEmptyOnNoInput confirms: empty slice / zero budget yield empty
// string (not a panic, not a stub header).
func TestRenderReturnsEmptyOnNoInput(t *testing.T) {
	if got := agentdoc.Render(nil, agentdoc.DefaultMaxBytes); got != "" {
		t.Fatalf("Render(nil) = %q", got)
	}
	if got := agentdoc.Render([]agentdoc.File{{Path: "/x", Content: "y"}}, 0); got != "" {
		t.Fatalf("Render(..., 0) = %q", got)
	}
}
