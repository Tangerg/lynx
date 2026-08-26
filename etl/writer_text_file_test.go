package etl_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/etl"
)

func TestTextFileWriterDefaultsToTextAndSupportsAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	first, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("first", nil)
	if writeErr := first.Write(t.Context(), []*document.Document{doc}); writeErr != nil {
		t.Fatal(writeErr)
	}

	second, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{
		Path: path, Append: true, DocumentMarkers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ = document.NewDocument("second", nil)
	if writeErr := second.Write(t.Context(), []*document.Document{doc}); writeErr != nil {
		t.Fatal(writeErr)
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

func TestTextFileWriterRequiresPath(t *testing.T) {
	if _, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{}); err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestTextFileWriterPreservesEncodedPageRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	writer, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{
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
	if writeErr := writer.Write(t.Context(), []*document.Document{doc}); writeErr != nil {
		t.Fatal(writeErr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.HasPrefix(got, "### Index: 0, Pages:[9007199254740993,9007199254740994]\n") {
		t.Fatalf("file contents = %q", got)
	}
}

func TestTextFileWriterRejectsTypedNilFormatter(t *testing.T) {
	var formatter *etl.SimpleFormatter
	if _, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{
		Path:      filepath.Join(t.TempDir(), "documents.txt"),
		Formatter: formatter,
	}); err == nil {
		t.Fatal("expected typed nil formatter error")
	}
}

func TestTextFileWriterHonorsCanceledContextBeforeOpening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "documents.txt")
	writer, err := etl.NewTextFileWriter(etl.TextFileWriterConfig{Path: path})
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
