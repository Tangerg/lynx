package toolset

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatJSONWritesIndentedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := formatPath(t.Context(), path); err != nil {
		t.Fatalf("formatPath: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "{\n  \"b\": 1,\n  \"a\": 2\n}\n"
	if string(got) != want {
		t.Fatalf("formatted JSON = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestFormatGoUsesBoundedInProcessFormatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main(){println(\"ok\")}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := formatPath(t.Context(), path); err != nil {
		t.Fatalf("formatPath: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "package main\n\nfunc main() { println(\"ok\") }\n"
	if string(got) != want {
		t.Fatalf("formatted Go = %q, want %q", got, want)
	}
}

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

func TestFormatPathRefusesOversizedSupportedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	content := `{"value":"` + strings.Repeat("x", (8<<20)+1) + `"}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	err := formatPath(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatalf("format oversized JSON error = %v, want explicit 8 MiB refusal", err)
	}
}

func TestRunFormatterBoundsDiagnosticOutput(t *testing.T) {
	_, err := runFormatter(
		t.Context(),
		nil,
		"/bin/sh",
		"-c",
		"/usr/bin/yes x | /usr/bin/head -c 131072 >&2; exit 1",
		"fixture.ts",
	)
	if err == nil {
		t.Fatal("runFormatter error = nil, want formatter failure")
	}
	if len(err.Error()) > (64<<10)+1024 {
		t.Fatalf("formatter diagnostic uses %d bytes, want bounded material", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("formatter error = %q, want honest truncation marker", err)
	}
}

func TestFormatOutputBufferCannotBypassLimitThroughIOCopy(t *testing.T) {
	buffer := &formatOutputBuffer{limit: 4}
	written, err := io.Copy(buffer, strings.NewReader("oversized"))
	if err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if written != int64(len("oversized")) {
		t.Fatalf("io.Copy wrote %d bytes, want a full drain", written)
	}
	if got := string(buffer.Bytes()); got != "over" || !buffer.overflow {
		t.Fatalf("buffer = %q, overflow = %t; want bounded drain", got, buffer.overflow)
	}
}

func TestRunFormatterPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := runFormatter(ctx, nil, "gofmt", "-w", filepath.Join(t.TempDir(), "file.go"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("formatter error = %v, want context.Canceled", err)
	}
}
