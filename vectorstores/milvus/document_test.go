package milvus

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/document"
)

func TestValidateProviderDocumentsRejectsValuesThatDoNotFitSchema(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		doc  *document.Document
		want error
	}{
		"ID":      {doc: &document.Document{ID: strings.Repeat("i", maxIDLength+1), Text: "text"}, want: ErrDocumentIDTooLong},
		"content": {doc: &document.Document{ID: "id", Text: strings.Repeat("t", maxContentLength+1)}, want: ErrDocumentContentTooLong},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateProviderDocuments([]*document.Document{test.doc}); !errors.Is(err, test.want) {
				t.Fatalf("validateProviderDocuments() error = %v, want %v", err, test.want)
			}
		})
	}
}
