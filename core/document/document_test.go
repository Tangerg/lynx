package document_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
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
	if doc.Formatter == nil {
		t.Fatal("Formatter must default to a non-nil instance")
	}
}

func TestNop_Methods(t *testing.T) {
	n := document.NewNop()
	doc, _ := document.NewDocument("hi", nil)

	if got, _ := n.Read(context.Background()); got != nil {
		t.Fatalf("Read = %v, want nil", got)
	}
	if err := n.Write(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := n.Format(doc, document.MetadataModeAll); got != "hi" {
		t.Fatalf("Format = %q", got)
	}
	if got, _ := n.Transform(context.Background(), []*document.Document{doc}); len(got) != 1 {
		t.Fatalf("Transform len = %d", len(got))
	}
	if got, _ := n.Batch(context.Background(), []*document.Document{doc}); len(got) != 1 || len(got[0]) != 1 {
		t.Fatal("Batch shape unexpected")
	}
}

func TestSimpleFormatter_AllAndNone(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	doc.Metadata["k"] = "v"

	f := document.NewSimpleFormatter(nil)

	all := f.Format(doc, document.MetadataModeAll)
	if !strings.Contains(all, "k: v") || !strings.Contains(all, "body") {
		t.Fatalf("ModeAll output: %q", all)
	}

	none := f.Format(doc, document.MetadataModeNone)
	if strings.Contains(none, "k: v") {
		t.Fatalf("ModeNone leaked metadata: %q", none)
	}
}

func TestSimpleFormatter_ExcludeKeys(t *testing.T) {
	doc, _ := document.NewDocument("body", nil)
	doc.Metadata["public"] = "yes"
	doc.Metadata["secret"] = "hidden"

	f := document.NewSimpleFormatter(&document.SimpleFormatterConfig{
		ExcludedEmbedMetadataKeys: []string{"secret"},
	})

	embed := f.Format(doc, document.MetadataModeEmbed)
	if strings.Contains(embed, "secret") {
		t.Fatalf("excluded key leaked in embed mode: %q", embed)
	}
	if !strings.Contains(embed, "public") {
		t.Fatalf("public key missing from embed mode: %q", embed)
	}
}

func TestTextSplitter_DefaultSeparatorIsNewline(t *testing.T) {
	s := document.NewTextSplitter(nil)

	doc, _ := document.NewDocument("a\nb\nc", nil)
	got, err := s.Transform(context.Background(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3", len(got))
	}
}

func TestTextSplitter_PreservesMetadata(t *testing.T) {
	s := document.NewTextSplitter(&document.TextSplitterConfig{Separator: "|"})

	doc, _ := document.NewDocument("a|b", nil)
	doc.Metadata["src"] = "manual"

	got, _ := s.Transform(context.Background(), []*document.Document{doc})
	for i, chunk := range got {
		if chunk.Metadata["src"] != "manual" {
			t.Fatalf("chunk[%d] missing metadata", i)
		}
	}
}

func TestSplitter_RejectsNilConfig(t *testing.T) {
	if _, err := document.NewSplitter(nil); err == nil {
		t.Fatal("nil config must error")
	}
	if _, err := document.NewSplitter(&document.SplitterConfig{}); err == nil {
		t.Fatal("missing SplitFunc must error")
	}
}

func TestSplitter_PropagatesError(t *testing.T) {
	want := errors.New("split failed")
	s, _ := document.NewSplitter(&document.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) { return nil, want },
	})

	doc, _ := document.NewDocument("x", nil)
	if _, err := s.Transform(context.Background(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestSplitter_DropsEmptyChunks(t *testing.T) {
	s, _ := document.NewSplitter(&document.SplitterConfig{
		SplitFunc: func(context.Context, string) ([]string, error) {
			return []string{"a", "", "b"}, nil
		},
	})

	doc, _ := document.NewDocument("x", nil)
	got, _ := s.Transform(context.Background(), []*document.Document{doc})
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2 (empty dropped)", len(got))
	}
}
