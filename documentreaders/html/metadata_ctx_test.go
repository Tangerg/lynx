package html_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coremetadata "github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentreaders/html"
)

func TestWithMetadata_AppliedToEveryDocument(t *testing.T) {
	metadata := mustMetadata(t, map[string]any{"source": "page.html", "tenant": "acme"})
	r, err := html.NewReader(strings.NewReader(samplePage),
		html.WithMetadata(metadata),
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata["source"][0] = 'x'
	docs, err := r.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatal("expected a document")
	}
	for i, d := range docs {
		if metadataValue[string](t, d.Metadata, "source") != "page.html" || metadataValue[string](t, d.Metadata, "tenant") != "acme" {
			t.Fatalf("doc %d missing extra metadata: %v", i, d.Metadata)
		}
	}
}

func TestWithMetadata_RejectsInvalidMetadataAtConstruction(t *testing.T) {
	_, err := html.NewReader(
		strings.NewReader(samplePage),
		html.WithMetadata(coremetadata.Map{"broken": []byte("{")}),
	)
	if !errors.Is(err, coremetadata.ErrInvalidValue) {
		t.Fatalf("NewReader error = %v, want ErrInvalidValue", err)
	}
}

func mustMetadata(t *testing.T, values map[string]any) coremetadata.Map {
	t.Helper()
	metadata, err := coremetadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestRead_HonorsContextCancellation(t *testing.T) {
	r, _ := html.NewReader(strings.NewReader(samplePage))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Read(ctx); err == nil {
		t.Fatal("canceled context must produce an error")
	}
}
