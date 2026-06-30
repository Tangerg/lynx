package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// buildTree lays out a small non-git tree under t.TempDir() for the walk path
// (t.TempDir is outside any repo, so ListFiles takes the filesystem fallback).
func buildTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{
		"a.txt",
		"sub/b.go",
		"sub/c.go",
		"node_modules/dep/x.js", // backstop-excluded
		".git/HEAD",             // always excluded
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(entries []FileEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}

func TestListFiles_RecursiveSkipsBackstop(t *testing.T) {
	root := buildTree(t)
	got, _, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "sub/b.go", "sub/c.go"}
	slices.Sort(want)
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, want) {
		t.Fatalf("recursive = %v, want %v (node_modules/.git must be excluded)", gotP, want)
	}
}

func TestListFiles_IncludeIgnoredSurfacesBackstop(t *testing.T) {
	root := buildTree(t)
	got, _, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true, IncludeIgnored: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths(got), "node_modules/dep/x.js") {
		t.Fatalf("includeIgnored should surface node_modules, got %v", paths(got))
	}
	if slices.Contains(paths(got), ".git/HEAD") {
		t.Fatal(".git must stay excluded even with includeIgnored")
	}
}

func TestListFiles_OneLevelDirsThenFiles(t *testing.T) {
	root := buildTree(t)
	got, _, err := ListFiles(context.Background(), root, ListFilesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Root level: the `sub` dir (dirs sort first) then the `a.txt` file.
	if len(got) != 2 || got[0].Kind != EntryDir || got[0].Name != "sub" {
		t.Fatalf("level[0] = %+v, want dir sub", got)
	}
	if got[1].Kind != EntryFile || got[1].Name != "a.txt" {
		t.Fatalf("level[1] = %+v, want file a.txt", got[1])
	}
}

func TestListFiles_ScopedToSubdir(t *testing.T) {
	root := buildTree(t)
	got, _, err := ListFiles(context.Background(), root, ListFilesOptions{Path: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, []string{"sub/b.go", "sub/c.go"}) {
		t.Fatalf("sub level = %v", gotP)
	}
}

func TestListFiles_GlobFilters(t *testing.T) {
	root := buildTree(t)
	got, _, err := ListFiles(context.Background(), root, ListFilesOptions{Glob: "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, []string{"sub/b.go", "sub/c.go"}) {
		t.Fatalf("glob **/*.go = %v", gotP)
	}
}

func TestListFiles_LimitTruncates(t *testing.T) {
	root := buildTree(t)
	got, truncated, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !truncated {
		t.Fatalf("limit=1 → %d entries truncated=%v, want 1 + true", len(got), truncated)
	}
}
