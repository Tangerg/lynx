package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/scope/tools/fs"
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
	if unmarshalErr := json.Unmarshal([]byte(body), &response); unmarshalErr != nil {
		t.Fatalf("decode read response: %v", unmarshalErr)
	}
	if response.Content != first || response.StartLine != 1 || response.EndLine != 1 || response.TotalLines != 2 || !response.Truncated {
		t.Fatalf(
			"read response = {bytes:%d start:%d end:%d total:%d truncated:%t}, want first complete line and honest continuation",
			len(response.Content), response.StartLine, response.EndLine, response.TotalLines, response.Truncated,
		)
	}
	body, err = directTools(dir)[0].Call(t.Context(), `{"path":"large.txt","start_line":2,"max_lines":1}`)
	if err != nil {
		t.Fatalf("read continuation: %v", err)
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode read continuation: %v", err)
	}
	if response.Content != second || response.StartLine != 2 || response.EndLine != 2 || response.TotalLines != 2 || !response.Truncated {
		t.Fatalf(
			"continuation = {bytes:%d start:%d end:%d total:%d truncated:%t}, want second complete line",
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

func TestRuntimeReadNormalizesBOMAndCRLFWithoutLosingTrailingLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "windows.txt"), []byte("\xef\xbb\xbffirst\r\nsecond\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := directTools(dir)[0].Call(t.Context(), `{"path":"windows.txt"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var response fs.ReadResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if response.Content != "first\nsecond\n" || response.StartLine != 1 || response.EndLine != 3 || response.TotalLines != 3 || response.Truncated {
		t.Fatalf("normalized response = %+v", response)
	}
}

func TestRuntimeReadRejectsUnpageableLineAndInvalidUTF8(t *testing.T) {
	for _, test := range []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "oversized line", content: []byte(strings.Repeat("x", (1<<20)+1)), want: "line 1 exceeds the 1 MiB limit"},
		{name: "invalid UTF-8", content: []byte{'o', 'k', 0xff}, want: fs.ErrBinaryFile.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "file.txt"), test.content, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := directTools(dir)[0].Call(t.Context(), `{"path":"file.txt"}`)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("read error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRuntimeReadDoesNotChargeUTF8BOMToLineBudget(t *testing.T) {
	dir := t.TempDir()
	line := strings.Repeat("x", 1<<20)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("\xef\xbb\xbf"+line), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := directTools(dir)[0].Call(t.Context(), `{"path":"file.txt"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var response fs.ReadResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if response.Content != line || response.TotalLines != 1 || response.Truncated {
		t.Fatalf("BOM-boundary response = {bytes:%d total:%d truncated:%t}", len(response.Content), response.TotalLines, response.Truncated)
	}
}
