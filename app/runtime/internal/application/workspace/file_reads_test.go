package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fileReadPort struct {
	input  FileReadInput
	result FileReadResult
}

func (port *fileReadPort) List(context.Context, string, FileListOptions) ([]FileEntry, error) {
	return nil, nil
}

func (port *fileReadPort) Read(_ context.Context, _ string, input FileReadInput) (FileReadResult, error) {
	port.input = input
	return port.result, nil
}

func (port *fileReadPort) Grep(context.Context, string, GrepInput) (GrepResult, error) {
	return GrepResult{}, nil
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
