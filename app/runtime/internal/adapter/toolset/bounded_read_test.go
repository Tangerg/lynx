package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/tools/fs"
)

const testMaxRuntimeReadFileBytes = 8 << 20

func TestRuntimeReadRefusesOversizedFileBeforeMaterialization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oversized.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", testMaxRuntimeReadFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := directTools(dir)[0].Call(t.Context(), `{"path":"oversized.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatalf("read oversized file error = %v, want explicit 8 MiB refusal", err)
	}
}

func TestRuntimeReadReturnsOnlyCompleteLinesWithinDefaultBudget(t *testing.T) {
	dir := t.TempDir()
	first := strings.Repeat("a", 600<<10)
	second := strings.Repeat("b", 600<<10)
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), []byte(first+"\n"+second), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := directTools(dir)[0].Call(t.Context(), `{"path":"large.txt"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var response fs.ReadResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if response.Content != first || response.StartLine != 1 || response.EndLine != 1 || response.TotalLines != 2 || !response.Truncated {
		t.Fatalf(
			"read response = {bytes:%d start:%d end:%d total:%d truncated:%t}, want first complete line and honest continuation",
			len(response.Content), response.StartLine, response.EndLine, response.TotalLines, response.Truncated,
		)
	}
}

func TestRuntimeReadPreservesPreCanceledContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := directTools(dir)[0].Call(ctx, `{"path":"file.txt"}`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v, want context.Canceled", err)
	}
}
