package codebaseindex

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeSourceFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFilesFiltersIndexablePaths(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "package main")
	writeSourceFile(t, root, "assets/logo.png", "not source")
	writeSourceFile(t, root, "node_modules/pkg/index.ts", "ignored")

	files, truncated, err := (Source{}).Files(context.Background(), root)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if truncated {
		t.Fatal("small tree should not truncate")
	}
	if !slices.Contains(files, "main.go") {
		t.Fatalf("files = %v, want main.go", files)
	}
	if slices.Contains(files, "assets/logo.png") || slices.Contains(files, "node_modules/pkg/index.ts") {
		t.Fatalf("files = %v, want non-code/heavy dirs filtered", files)
	}
}

func TestSourceChunksSkipsBinary(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeSourceFile(t, root, "bad.go", "package main\x00")

	chunks, hash, ok := (Source{}).Chunks(root, "main.go")
	if !ok || hash == "" || len(chunks) != 1 {
		t.Fatalf("Chunks(main.go) = len %d hash %q ok %v", len(chunks), hash, ok)
	}
	if chunks[0].Path != "main.go" || chunks[0].StartLine != 1 || chunks[0].Text == "" {
		t.Fatalf("chunk = %+v", chunks[0])
	}
	if chunks, _, ok := (Source{}).Chunks(root, "bad.go"); ok || len(chunks) != 0 {
		t.Fatalf("binary chunks = %+v ok %v, want skipped", chunks, ok)
	}
}
