package attachment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func TestResolveClassifiesAndCanonicalizesWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "notes.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.Resolve(t.Context(), "docs/notes.md")
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(path)
	if got.ID == "" || got.Kind != client.AttachmentText || got.Name != "docs/notes.md" || got.Path != canonical || got.MimeType != "text/markdown" || got.Size != 8 {
		t.Fatalf("attachment = %+v", got)
	}
	again, err := resolver.Resolve(t.Context(), path)
	if err != nil || again.ID != got.ID {
		t.Fatalf("stable identity = %+v, %v; want %s", again, err, got.ID)
	}
}

func TestResolveRejectsDirectoriesAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	resolver, _ := New(root)
	if _, err := resolver.Resolve(t.Context(), root); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory error = %v", err)
	}
	resolver.maxBytes = 2
	path := filepath.Join(root, "large.txt")
	if err := os.WriteFile(path, []byte("large"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(t.Context(), path); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestCompleteRanksFilesAndSkipsDependencyInternals(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"internal/cache/store.go", "cache_test.go", ".git/cache-secret", "node_modules/cache.js"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolver, _ := New(root)
	got, err := resolver.Complete(t.Context(), "cache", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "cache_test.go" {
		t.Fatalf("matches = %+v", got)
	}
	for _, match := range got {
		if strings.Contains(match.Path, ".git") || strings.Contains(match.Path, "node_modules") {
			t.Fatalf("ignored path returned: %+v", match)
		}
	}
}

func TestCompleteHonorsCanceledContext(t *testing.T) {
	resolver, _ := New(t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resolver.Complete(ctx, "", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
