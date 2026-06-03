package markdown_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/documentreaders/markdown"
)

func TestWithMetadata_AppliedToEveryDocument(t *testing.T) {
	r, err := markdown.NewReader(strings.NewReader(sample),
		markdown.WithHeadingSplit(2),
		markdown.WithMetadata(map[string]any{"source": "manual.md", "tenant": "acme"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected sections")
	}
	for i, d := range docs {
		if d.Metadata["source"] != "manual.md" || d.Metadata["tenant"] != "acme" {
			t.Fatalf("doc %d missing extra metadata: %v", i, d.Metadata)
		}
	}
}

func TestWithMetadata_DoesNotClobberReaderKeys(t *testing.T) {
	// A user key colliding with a reader-namespaced key must not win.
	r, _ := markdown.NewReader(strings.NewReader(sample),
		markdown.WithHeadingSplit(1),
		markdown.WithMetadata(map[string]any{markdown.MetadataHeading: "HIJACK"}),
	)
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if h, ok := d.Metadata[markdown.MetadataHeading]; ok && h == "HIJACK" {
			t.Fatal("reader-derived heading must take precedence over extra metadata")
		}
	}
}

func TestRead_HonorsContextCancellation(t *testing.T) {
	r, _ := markdown.NewReader(strings.NewReader(sample), markdown.WithHeadingSplit(2))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Read(ctx); err == nil {
		t.Fatal("canceled context must produce an error")
	}
}
