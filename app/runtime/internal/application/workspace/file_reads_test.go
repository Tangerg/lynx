package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fileReadPort struct {
	input      FileReadInput
	result     FileReadResult
	grepInput  GrepPlan
	grepResult GrepResult
	grepCalled bool
}

func (f *fileReadPort) List(context.Context, string, FileListOptions) ([]FileEntry, error) {
	return nil, nil
}

func (f *fileReadPort) Read(_ context.Context, _ string, input FileReadInput) (FileReadResult, error) {
	f.input = input
	return f.result, nil
}

func (f *fileReadPort) Grep(_ context.Context, _ string, input GrepPlan) (GrepResult, error) {
	f.grepCalled = true
	f.grepInput = input
	return f.grepResult, nil
}

type fileReadPaths struct{}

func (fileReadPaths) ResolveExistingDir(path string) (string, error) { return path, nil }
func (fileReadPaths) ResolveInRoot(_ string, path string) (string, error) {
	return path, nil
}
func (fileReadPaths) ResolveExistingInRoot(_ string, path string) (string, error) {
	return path, nil
}

func TestFilesReadNormalizesBudgetBeforeCallingPort(t *testing.T) {
	port := &fileReadPort{result: FileReadResult{Content: "text", TotalLines: 1, EndLine: 1}}
	files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

	got, err := files.Read(t.Context(), "", FileReadInput{Path: "file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "text" || port.input.MaxBytes != DefaultFileReadBytes {
		t.Fatalf("Read = %+v, port budget = %d", got, port.input.MaxBytes)
	}
}

func TestFilesReadRejectsInvalidPortResults(t *testing.T) {
	tests := []struct {
		name   string
		result FileReadResult
		want   error
	}{
		{
			name:   "oversized output",
			result: FileReadResult{Content: strings.Repeat("x", DefaultFileReadBytes+1), TotalLines: 1, EndLine: 1},
			want:   ErrFileReadTooLarge,
		},
		{
			name:   "invalid text",
			result: FileReadResult{Content: string([]byte{0xff}), TotalLines: 1, EndLine: 1},
			want:   ErrUnsupportedText,
		},
		{
			name:   "unmarked omission",
			result: FileReadResult{Content: "first", TotalLines: 2, EndLine: 1},
		},
		{
			name:   "content outside window",
			result: FileReadResult{Content: "first\nsecond", TotalLines: 2, EndLine: 1, Truncated: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := &fileReadPort{result: test.result}
			files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)
			_, err := files.Read(t.Context(), "", FileReadInput{Path: "file.txt"})
			if err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Read error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFilesHeadRejectsByteTruncatedPortResult(t *testing.T) {
	port := &fileReadPort{result: FileReadResult{
		Content: "prefix", TotalLines: 2, EndLine: 1, Truncated: true, OutputTruncated: true,
	}}
	files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

	_, err := files.Head(t.Context(), "", "file.txt", 1)
	if !errors.Is(err, ErrFileReadTooLarge) {
		t.Fatalf("Head error = %v, want ErrFileReadTooLarge", err)
	}
}

func TestFilesGrepOwnsQueryLimitAndPortResultEnvelope(t *testing.T) {
	t.Run("invalid regex", func(t *testing.T) {
		port := &fileReadPort{}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "["}); err == nil {
			t.Fatal("Grep accepted an invalid regular expression")
		}
		if port.grepCalled {
			t.Fatal("Grep called the filesystem port before validating the query")
		}
	})

	t.Run("oversized query", func(t *testing.T) {
		port := &fileReadPort{}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: strings.Repeat("x", (64<<10)+1)}); err == nil {
			t.Fatal("Grep accepted a query larger than 64 KiB")
		}
		if port.grepCalled {
			t.Fatal("Grep called the filesystem port with an oversized query")
		}
	})

	t.Run("caller limit", func(t *testing.T) {
		port := &fileReadPort{}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "needle", Limit: 100_000}); err != nil {
			t.Fatal(err)
		}
		if port.grepInput.Limit != 1000 {
			t.Fatalf("port limit = %d, want Application-owned maximum 1000", port.grepInput.Limit)
		}
	})

	t.Run("dishonest direct result", func(t *testing.T) {
		port := &fileReadPort{grepResult: GrepResult{
			Matches: []GrepMatch{
				{Path: "a.go", LineNumber: 1, Text: "needle"},
				{Path: "b.go", LineNumber: 1, Text: "needle"},
			},
			Total: 1,
		}}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "needle", Limit: 1}); err == nil {
			t.Fatal("Grep published a direct-port result beyond its match limit and total")
		}
	})

	t.Run("oversized direct material", func(t *testing.T) {
		line := "needle" + strings.Repeat("x", (1<<20)-len("needle"))
		matches := make([]GrepMatch, 9)
		for index := range matches {
			matches[index] = GrepMatch{Path: "file.go", LineNumber: index + 1, Text: line}
		}
		port := &fileReadPort{grepResult: GrepResult{Matches: matches, Total: len(matches)}}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		_, err := files.Grep(t.Context(), "", GrepInput{Query: "needle", Limit: len(matches)})
		if !errors.Is(err, ErrGrepResultTooLarge) {
			t.Fatalf("Grep oversized direct result error = %v, want ErrGrepResultTooLarge", err)
		}
	})

	t.Run("unsafe direct path", func(t *testing.T) {
		port := &fileReadPort{grepResult: GrepResult{
			Matches: []GrepMatch{{Path: "../secret", LineNumber: 1, Text: "needle"}}, Total: 1,
		}}
		files := NewFiles(NewScope(t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "needle"}); err == nil {
			t.Fatal("Grep published a direct-port path outside the workspace")
		}
	})
}
