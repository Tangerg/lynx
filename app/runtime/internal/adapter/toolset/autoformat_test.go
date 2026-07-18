package toolset

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatPathIgnoresDeletedFile(t *testing.T) {
	if err := formatPath(t.Context(), filepath.Join(t.TempDir(), "deleted.go")); err != nil {
		t.Fatalf("format deleted file: %v", err)
	}
}

func TestFormatPathSurfacesUnexpectedStatFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := formatPath(t.Context(), filepath.Join(parent, "child.go"))
	if err == nil || !strings.Contains(err.Error(), "inspect before formatting") {
		t.Fatalf("format error = %v, want stat context", err)
	}
}

func TestRunFormatterPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := runFormatter(ctx, "gofmt", "-w", filepath.Join(t.TempDir(), "file.go"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("formatter error = %v, want context.Canceled", err)
	}
}
