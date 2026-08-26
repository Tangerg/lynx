package markdown_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	coremetadata "github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/etl/markdown"
)

func TestConfigMetadataAppliedToEveryDocument(t *testing.T) {
	metadata := mustMetadata(t, map[string]any{"source": "manual.md", "tenant": "acme"})
	r, err := markdown.New(strings.NewReader(sample),
		markdown.Config{HeadingSplitLevel: 2, Metadata: metadata},
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
		t.Fatal("expected sections")
	}
	for i, d := range docs {
		source, _ := metadataValue[string](t, d.Metadata, "source")
		tenant, _ := metadataValue[string](t, d.Metadata, "tenant")
		if source != "manual.md" || tenant != "acme" {
			t.Fatalf("doc %d missing extra metadata: %v", i, d.Metadata)
		}
	}
}

func TestConfigMetadataDoesNotClobberReaderKeys(t *testing.T) {
	// A user key colliding with a reader-namespaced key must not win.
	r, _ := markdown.New(strings.NewReader(sample),
		markdown.Config{
			HeadingSplitLevel: 1,
			Metadata:          mustMetadata(t, map[string]any{markdown.MetadataHeading: "HIJACK"}),
		},
	)
	docs, err := r.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if h, ok := metadataValue[string](t, d.Metadata, markdown.MetadataHeading); ok && h == "HIJACK" {
			t.Fatal("reader-derived heading must take precedence over extra metadata")
		}
	}
}

func TestConfigMetadataRejectsInvalidValueAtConstruction(t *testing.T) {
	_, err := markdown.New(
		strings.NewReader(sample),
		markdown.Config{Metadata: coremetadata.Map{"broken": []byte("{")}},
	)
	if !errors.Is(err, coremetadata.ErrInvalidValue) {
		t.Fatalf("New error = %v, want ErrInvalidValue", err)
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
	r, _ := markdown.New(strings.NewReader(sample), markdown.Config{HeadingSplitLevel: 2})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Read(ctx); err == nil {
		t.Fatal("canceled context must produce an error")
	}
}
