package docio_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
)

func TestValidateDocuments(t *testing.T) {
	valid := func(id string) *document.Document {
		return &document.Document{ID: id, Text: "content"}
	}

	tests := []struct {
		name string
		docs []*document.Document
		want error
	}{
		{name: "empty", want: vectorstore.ErrEmptyDocuments},
		{name: "nil document", docs: []*document.Document{nil}, want: vectorstore.ErrInvalidDocument},
		{name: "missing content", docs: []*document.Document{{ID: "one"}}, want: vectorstore.ErrInvalidDocument},
		{name: "missing ID", docs: []*document.Document{{Text: "content"}}, want: vectorstore.ErrMissingDocumentID},
		{name: "blank ID", docs: []*document.Document{{ID: "  ", Text: "content"}}, want: vectorstore.ErrMissingDocumentID},
		{name: "duplicate ID", docs: []*document.Document{valid("one"), valid("one")}, want: vectorstore.ErrDuplicateDocumentID},
		{name: "valid", docs: []*document.Document{valid("one"), valid("two")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := docio.ValidateDocuments(test.docs)
			if !errors.Is(err, test.want) {
				t.Fatalf("ValidateDocuments() error = %v, want %v", err, test.want)
			}
		})
	}
}
