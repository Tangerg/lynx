package documentpipeline_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentpipeline"
)

func TestNewDocument_RequiresContent(t *testing.T) {
	if _, err := document.NewDocument("", nil); err == nil {
		t.Fatal("empty doc must error")
	}
}

func TestNewDocument_AllocatesMetadata(t *testing.T) {
	doc, err := document.NewDocument("hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Metadata == nil {
		t.Fatal("Metadata must be allocated")
	}
}

func TestFunctionAdapters(t *testing.T) {
	doc, _ := document.NewDocument("hi", nil)
	docs := []*document.Document{doc}

	formatter := documentpipeline.FormatterFunc(func(doc *document.Document, _ documentpipeline.MetadataMode) (string, error) {
		return strings.ToUpper(doc.Text), nil
	})
	if got, _ := formatter.Format(doc, documentpipeline.MetadataModeAll); got != "HI" {
		t.Fatalf("Format = %q", got)
	}

	transformer := documentpipeline.TransformerFunc(func(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
		return docs, nil
	})
	if got, _ := transformer.Transform(t.Context(), docs); len(got) != 1 {
		t.Fatalf("Transform len = %d", len(got))
	}

	batcher := documentpipeline.BatcherFunc(func(_ context.Context, docs []*document.Document) ([][]*document.Document, error) {
		return [][]*document.Document{docs}, nil
	})
	if got, _ := batcher.Batch(t.Context(), docs); len(got) != 1 || len(got[0]) != 1 {
		t.Fatal("Batch shape unexpected")
	}
}

func TestBoundFormatterDefaultsToDocumentText(t *testing.T) {
	doc, _ := document.NewDocument("hi", nil)
	got, err := (documentpipeline.BoundFormatter{}).Format(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi" {
		t.Fatalf("Format = %q, want hi", got)
	}
}

func TestSimpleFormatter_AllAndNone(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	_ = doc.Metadata.Set("k", "v")

	f := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{})

	all, err := f.Format(doc, documentpipeline.MetadataModeAll)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all, "k: v") || !strings.Contains(all, "body") {
		t.Fatalf("ModeAll output: %q", all)
	}

	none, err := f.Format(doc, documentpipeline.MetadataModeNone)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none, "k: v") {
		t.Fatalf("ModeNone leaked metadata: %q", none)
	}
}

func TestSimpleFormatter_ExcludeKeys(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	_ = doc.Metadata.Set("public", "yes")
	_ = doc.Metadata.Set("secret", "hidden")

	excluded := []string{"secret"}
	f := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{
		ExcludeFromEmbedding: excluded,
	})
	excluded[0] = "public"

	embed, err := f.Format(doc, documentpipeline.MetadataModeEmbed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(embed, "secret") {
		t.Fatalf("excluded key leaked in embed mode: %q", embed)
	}
	if !strings.Contains(embed, "public") {
		t.Fatalf("public key missing from embed mode: %q", embed)
	}
}

func TestSimpleFormatter_PreservesTypedMetadataBoundary(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	doc.Metadata = metadata.Map{
		"null":   []byte("null"),
		"number": []byte("9007199254740993"),
		"object": []byte(`{ "nested": true }`),
		"string": []byte(`"plain"`),
	}

	formatted, err := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{}).Format(
		doc,
		documentpipeline.MetadataModeAll,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "null: \nnumber: 9007199254740993\nobject: {\"nested\":true}\nstring: plain\n\nbody"
	if formatted != want {
		t.Fatalf("Format = %q, want %q", formatted, want)
	}
}

func TestSimpleFormatter_RejectsInvalidEncodedMetadata(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	doc.Metadata = metadata.Map{"broken": []byte("{")}

	_, err := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{}).Format(
		doc,
		documentpipeline.MetadataModeAll,
	)
	if !errors.Is(err, metadata.ErrInvalidValue) {
		t.Fatalf("Format error = %v, want ErrInvalidValue", err)
	}
}

func TestTextSplitter_DefaultSeparatorIsNewline(t *testing.T) {
	s, err := documentpipeline.NewTextSplitter(documentpipeline.TextSplitterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("a\nb\nc", nil)
	got, err := s.Transform(t.Context(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3", len(got))
	}
}

func TestTextSplitter_PreservesMetadata(t *testing.T) {
	s, err := documentpipeline.NewTextSplitter(documentpipeline.TextSplitterConfig{Separator: "|"})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("a|b", nil)
	_ = doc.Metadata.Set("src", "manual")

	got, _ := s.Transform(t.Context(), []*document.Document{doc})
	for i, chunk := range got {
		if src, ok, _ := metadata.Decode[string](chunk.Metadata, "src"); !ok || src != "manual" {
			t.Fatalf("chunk[%d] missing metadata", i)
		}
	}
}

func TestSplitter_RejectsMissingSplitFunc(t *testing.T) {
	if _, err := documentpipeline.NewSplitter(documentpipeline.SplitterConfig{}); err == nil {
		t.Fatal("missing SplitFunc must error")
	}
}

func TestSplitter_PropagatesError(t *testing.T) {
	want := errors.New("split failed")
	s, _ := documentpipeline.NewSplitter(documentpipeline.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) { return nil, want },
	})

	doc, _ := document.NewDocument("x", nil)
	if _, err := s.Transform(t.Context(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestSplitter_DropsEmptyChunks(t *testing.T) {
	s, _ := documentpipeline.NewSplitter(documentpipeline.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) {
			return []string{"a", "", "b"}, nil
		},
	})

	doc, _ := document.NewDocument("x", nil)
	got, _ := s.Transform(t.Context(), []*document.Document{doc})
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2 (empty dropped)", len(got))
	}
}

func TestStagesRejectNilDocuments(t *testing.T) {
	splitter, err := documentpipeline.NewSplitter(documentpipeline.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) { return []string{"chunk"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := splitter.Transform(t.Context(), []*document.Document{nil}); !errors.Is(err, documentpipeline.ErrNilDocument) {
		t.Fatalf("Splitter error = %v, want ErrNilDocument", err)
	}

	if _, err := (documentpipeline.BoundFormatter{}).Format(nil); !errors.Is(err, documentpipeline.ErrNilDocument) {
		t.Fatalf("BoundFormatter error = %v, want ErrNilDocument", err)
	}
}

func TestFormatterRejectsUnknownMetadataMode(t *testing.T) {
	doc, _ := document.NewDocument("text", nil)
	formatter := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{})
	if _, err := formatter.Format(doc, "mystery"); !errors.Is(err, documentpipeline.ErrInvalidMetadataMode) {
		t.Fatalf("Format() error = %v, want ErrInvalidMetadataMode", err)
	}
}
