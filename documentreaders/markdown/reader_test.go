package markdown_test

import (
	"context"
	"strings"
	"testing"

	coremetadata "github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentreaders/markdown"
)

func metadataValue[T any](t *testing.T, values coremetadata.Map, key string) (T, bool) {
	t.Helper()
	value, ok, err := coremetadata.Decode[T](values, key)
	if err != nil {
		t.Fatalf("metadata %q: %v", key, err)
	}
	return value, ok
}

const sample = `# Intro

Some intro paragraph with **bold**.

## Section A

Body of section A.

### A.1 subsection

Nested body — should stay inside A when split at H2.

## Section B

Body of section B.
`

func TestWholeDocument(t *testing.T) {
	r, err := markdown.NewReader(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("whole mode: want 1 doc, got %d", len(docs))
	}
	if !strings.Contains(docs[0].Text, "Section B") {
		t.Errorf("whole-doc body missing Section B; got: %q", docs[0].Text)
	}
}

func TestHeadingSplitH2(t *testing.T) {
	r, err := markdown.NewReader(
		strings.NewReader(sample),
		markdown.WithHeadingSplit(2),
		markdown.WithSourceName("test.md"),
	)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Expect 3 sections: Intro (H1), Section A (H2, includes A.1
	// subsection at H3), Section B (H2).
	if len(docs) != 3 {
		t.Fatalf("split mode: want 3 docs, got %d", len(docs))
	}

	want := []struct {
		heading string
		path    string
	}{
		{"Intro", "Intro"},
		{"Section A", "Intro > Section A"},
		{"Section B", "Intro > Section B"},
	}
	for i, w := range want {
		if got, _ := metadataValue[string](t, docs[i].Metadata, markdown.MetadataHeading); got != w.heading {
			t.Errorf("docs[%d] heading: want %q, got %v", i, w.heading, got)
		}
		if got, _ := metadataValue[string](t, docs[i].Metadata, markdown.MetadataHeadingPath); got != w.path {
			t.Errorf("docs[%d] path: want %q, got %v", i, w.path, got)
		}
		if got, _ := metadataValue[string](t, docs[i].Metadata, markdown.MetadataSourceName); got != "test.md" {
			t.Errorf("docs[%d] source: want test.md, got %v", i, got)
		}
	}

	// Section A should contain A.1 subsection content.
	if !strings.Contains(docs[1].Text, "Nested body") {
		t.Errorf("section A missing nested H3 body; got: %q", docs[1].Text)
	}
}
