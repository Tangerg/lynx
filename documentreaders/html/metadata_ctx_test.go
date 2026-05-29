package html_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/documentreaders/html"
)

func TestWithMetadata_AppliedToEveryDocument(t *testing.T) {
	r, err := html.NewReader(strings.NewReader(samplePage),
		html.WithMetadata(map[string]any{"source": "page.html", "tenant": "acme"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected a document")
	}
	for i, d := range docs {
		if d.Metadata["source"] != "page.html" || d.Metadata["tenant"] != "acme" {
			t.Fatalf("doc %d missing extra metadata: %v", i, d.Metadata)
		}
	}
}

func TestRead_HonorsContextCancellation(t *testing.T) {
	r, _ := html.NewReader(strings.NewReader(samplePage))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Read(ctx); err == nil {
		t.Fatal("canceled context must produce an error")
	}
}
