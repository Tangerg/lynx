package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIDFromFileUsesSHA256ContentDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra")
	content := []byte("runtime executable")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write executable fixture: %v", err)
	}

	got, err := buildIDFromFile(path)
	if err != nil {
		t.Fatalf("buildIDFromFile: %v", err)
	}
	sum := sha256.Sum256(content)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("BuildID = %q, want %q", got, want)
	}
}

func TestBuildIDFromFileReportsReadFailure(t *testing.T) {
	_, err := buildIDFromFile(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !strings.Contains(err.Error(), "build identity") {
		t.Fatalf("buildIDFromFile error = %v, want contextual read failure", err)
	}
}
