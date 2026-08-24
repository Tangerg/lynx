package lsp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureOpenRejectsOversizedDocumentBeforeNotification(t *testing.T) {
	const app2DocumentLimit = 8 << 20

	content := bytes.Repeat([]byte("x"), app2DocumentLimit+1)
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(path)
	client := &client{
		spec: ServerSpec{LanguageID: "go"},
		open: map[string]openDoc{
			uri: {version: 1, hash: sha256.Sum256(content)},
		},
	}

	if version, err := client.ensureOpen(t.Context(), path); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("ensureOpen = (%d, %v), want ErrDocumentTooLarge", version, err)
	}
	if client.open[uri].version != 1 {
		t.Fatalf("rejected sync changed open document state: %+v", client.open[uri])
	}
}

func TestReadDocumentHonorsExactBoundaryAndCancellation(t *testing.T) {
	content := bytes.Repeat([]byte("x"), int(maxDocumentBytes))
	path := filepath.Join(t.TempDir(), "boundary.go")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := readDocument(t.Context(), path)
	if err != nil {
		t.Fatalf("read exact boundary: %v", err)
	}
	if len(read) != len(content) {
		t.Fatalf("read %d bytes, want %d", len(read), len(content))
	}

	canceled, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("stop document read")
	cancel(cause)
	if _, err := readDocument(canceled, path); !errors.Is(err, cause) {
		t.Fatalf("canceled read error = %v, want %v", err, cause)
	}
}
