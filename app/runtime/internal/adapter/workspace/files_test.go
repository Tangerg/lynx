package workspace

import (
	"context"
	"errors"
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
	got, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true})
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
	got, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true, IncludeIgnored: true})
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
	got, err := ListFiles(context.Background(), root, ListFilesOptions{})
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

func TestListFiles_OneLevelIncludesEmptyDirectoryWithoutDescending(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ListFiles(context.Background(), root, ListFilesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"empty", "a.txt"}; !slices.Equal(paths(got), want) {
		t.Fatalf("paths = %v, want %v", paths(got), want)
	}
}

func TestListFiles_HidesGitControlFileAndBoundsOneLevelReads(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".git", "a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, options := range []ListFilesOptions{
		{IncludeIgnored: true},
		{Recursive: true, IncludeIgnored: true},
	} {
		got, err := ListFiles(context.Background(), root, options)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(paths(got), ".git") {
			t.Fatalf("ListFiles(%+v) exposed the Git control file: %v", options, paths(got))
		}
	}

	if _, err := readDirectoryEntries(root, 3); !errors.Is(err, ErrListingTooLarge) {
		t.Fatalf("readDirectoryEntries() error = %v, want ErrListingTooLarge", err)
	}
}

func TestListFiles_ScopedToSubdir(t *testing.T) {
	root := buildTree(t)
	got, err := ListFiles(context.Background(), root, ListFilesOptions{Path: "sub"})
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
	got, err := ListFiles(context.Background(), root, ListFilesOptions{Glob: "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	gotP := paths(got)
	slices.Sort(gotP)
	if !slices.Equal(gotP, []string{"sub/b.go", "sub/c.go"}) {
		t.Fatalf("glob **/*.go = %v", gotP)
	}
}

func TestListFilesGlobUsesDoublestarRelativeToSelectedPath(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"project/src/root.ts",
		"project/src/nested/deep.ts",
		"project/src/nested/deep.go",
		"project/other.ts",
	} {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListFiles(context.Background(), root, ListFilesOptions{
		Path: "project", Glob: "src/**/*.ts",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPaths := paths(got); !slices.Equal(gotPaths, []string{
		"project/src/nested/deep.ts",
		"project/src/root.ts",
	}) {
		t.Fatalf("scoped doublestar paths = %v", gotPaths)
	}
}

func TestListFilesRejectsInvalidGlob(t *testing.T) {
	t.Parallel()

	_, err := ListFiles(context.Background(), t.TempDir(), ListFilesOptions{Glob: "["})
	if !errors.Is(err, ErrInvalidGlob) {
		t.Fatalf("ListFiles() error = %v, want ErrInvalidGlob", err)
	}
}

func TestListFilesInspectsMetadataAndSymlinks(t *testing.T) {
	root := buildTree(t)
	if err := os.Symlink("a.txt", filepath.Join(root, "a-link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	got, err := ListFiles(context.Background(), root, ListFilesOptions{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	var file, link FileEntry
	for _, entry := range got {
		switch entry.Path {
		case "a.txt":
			file = entry
		case "a-link":
			link = entry
		}
	}
	if file.Kind != EntryFile || file.SizeBytes != 1 || file.ModifiedAt.IsZero() {
		t.Fatalf("file metadata = %+v", file)
	}
	if link.Kind != EntrySymlink {
		t.Fatalf("symlink metadata = %+v", link)
	}
}

func TestListFilesHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ListFiles(ctx, t.TempDir(), ListFilesOptions{Recursive: true, IncludeIgnored: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ListFiles() error = %v, want context.Canceled", err)
	}
}
