package lsp

import (
	"bytes"
	"crypto/sha256"
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

	if version, err := client.ensureOpen(t.Context(), path); err == nil {
		t.Fatalf("ensureOpen accepted an oversized document at version %d", version)
	}
}
