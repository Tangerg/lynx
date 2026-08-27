package etl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/etl"
)

func TestTextSplitterDefaultsToNewline(t *testing.T) {
	splitter, err := etl.NewTextSplitter(etl.TextSplitterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	texts, err := splitter.SplitText(t.Context(), "a\nb\nc")
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 3 {
		t.Fatalf("SplitText returned %d chunks, want 3", len(texts))
	}

	doc, _ := document.NewDocument("a\nb\nc", nil)
	docs, err := splitter.Split(t.Context(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("Split returned %d chunks, want 3", len(docs))
	}
}

func TestTextSplitterPreservesMetadata(t *testing.T) {
	splitter, err := etl.NewTextSplitter(etl.TextSplitterConfig{Separator: "|"})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("a|b", nil)
	_ = doc.Metadata.Set("src", "manual")
	docs, _ := splitter.Split(t.Context(), []*document.Document{doc})
	for index, chunk := range docs {
		if src, ok, _ := chunk.Metadata.Decode[string]("src"); !ok || src != "manual" {
			t.Fatalf("chunk[%d] missing metadata", index)
		}
	}
}

func TestSplitterRequiresSplitFunc(t *testing.T) {
	if _, err := etl.NewSplitter(etl.SplitterConfig{}); err == nil {
		t.Fatal("missing SplitFunc must error")
	}
}

func TestSplitterPropagatesError(t *testing.T) {
	want := errors.New("split failed")
	splitter, _ := etl.NewSplitter(etl.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) { return nil, want },
	})
	doc, _ := document.NewDocument("x", nil)

	if _, err := splitter.Split(t.Context(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("Split error = %v, want %v", err, want)
	}
}

func TestSplitterDropsEmptyChunks(t *testing.T) {
	splitter, _ := etl.NewSplitter(etl.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) {
			return []string{"a", "", "b"}, nil
		},
	})
	doc, _ := document.NewDocument("x", nil)

	docs, _ := splitter.Split(t.Context(), []*document.Document{doc})
	if len(docs) != 2 {
		t.Fatalf("Split returned %d chunks, want 2", len(docs))
	}
}

func TestSplitterRejectsNilDocument(t *testing.T) {
	splitter, err := etl.NewSplitter(etl.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) { return []string{"chunk"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := splitter.Split(t.Context(), []*document.Document{nil}); !errors.Is(err, etl.ErrNilDocument) {
		t.Fatalf("Split error = %v, want ErrNilDocument", err)
	}
}
