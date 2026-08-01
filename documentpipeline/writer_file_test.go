package documentpipeline_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentpipeline"
)

func TestFileWriterDefaultsToTextAndSupportsAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	first, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("first", nil)
	if err := first.Write(t.Context(), []*document.Document{doc}); err != nil {
		t.Fatal(err)
	}

	second, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{
		Path: path, Append: true, DocumentMarkers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ = document.NewDocument("second", nil)
	if err := second.Write(t.Context(), []*document.Document{doc}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "first\n\n") || !strings.Contains(text, "### Index: 0\nsecond\n\n") {
		t.Fatalf("file contents = %q", text)
	}
}

func TestFileWriterRequiresPath(t *testing.T) {
	if _, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{}); err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestFileWriterPreservesEncodedPageRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	writer, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{
		Path:            path,
		DocumentMarkers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("body", nil)
	doc.Metadata = metadata.Map{
		"start_page_number": []byte("9007199254740993"),
		"end_page_number":   []byte("9007199254740994"),
	}
	if err := writer.Write(t.Context(), []*document.Document{doc}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.HasPrefix(got, "### Index: 0, Pages:[9007199254740993,9007199254740994]\n") {
		t.Fatalf("file contents = %q", got)
	}
}

func TestFileWriterRejectsInvalidMode(t *testing.T) {
	if _, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{
		Path: filepath.Join(t.TempDir(), "documents.txt"),
		Mode: "unknown",
	}); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestFileWriterHonorsCanceledContextBeforeOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	writer, err := documentpipeline.NewFileWriter(documentpipeline.FileWriterConfig{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := writer.Write(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file stat error = %v, want os.ErrNotExist", err)
	}
}
