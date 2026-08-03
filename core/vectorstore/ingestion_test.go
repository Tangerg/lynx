package vectorstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
)

type batcherFunc func(context.Context, []*document.Document) ([][]*document.Document, error)

func (f batcherFunc) Batch(
	ctx context.Context,
	documents []*document.Document,
) ([][]*document.Document, error) {
	return f(ctx, documents)
}

func TestValidateDocuments(t *testing.T) {
	valid := func(id string) *document.Document {
		return &document.Document{ID: id, Text: "content"}
	}
	tests := []struct {
		name      string
		documents []*document.Document
		want      error
	}{
		{name: "empty", want: vectorstore.ErrEmptyDocuments},
		{name: "nil document", documents: []*document.Document{nil}, want: vectorstore.ErrInvalidDocument},
		{name: "missing content", documents: []*document.Document{{ID: "one"}}, want: vectorstore.ErrInvalidDocument},
		{name: "missing ID", documents: []*document.Document{{Text: "content"}}, want: vectorstore.ErrMissingDocumentID},
		{name: "blank ID", documents: []*document.Document{{ID: "  ", Text: "content"}}, want: vectorstore.ErrMissingDocumentID},
		{name: "duplicate ID", documents: []*document.Document{valid("one"), valid("one")}, want: vectorstore.ErrDuplicateDocumentID},
		{name: "valid", documents: []*document.Document{valid("one"), valid("two")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := vectorstore.ValidateDocuments(test.documents); !errors.Is(err, test.want) {
				t.Fatalf("ValidateDocuments() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestBatchDocuments(t *testing.T) {
	documents := []*document.Document{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	batches, err := vectorstore.BatchDocuments(t.Context(), batcherFunc(func(
		_ context.Context,
		input []*document.Document,
	) ([][]*document.Document, error) {
		return [][]*document.Document{input[:2], input[2:]}, nil
	}), documents)
	if err != nil {
		t.Fatalf("BatchDocuments: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(batches))
	}
}

func TestBatchDocumentsRejectsInvalidPartitions(t *testing.T) {
	documents := []*document.Document{{ID: "a"}, {ID: "b"}}
	tests := map[string][][]*document.Document{
		"missing document": {documents[:1]},
		"reordered":        {{documents[1], documents[0]}},
		"duplicate":        {{documents[0], documents[0]}},
		"empty batch":      {documents[:1], nil, documents[1:]},
		"extra document":   {documents, {{ID: "c"}}},
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := vectorstore.BatchDocuments(t.Context(), batcherFunc(func(
				context.Context,
				[]*document.Document,
			) ([][]*document.Document, error) {
				return output, nil
			}), documents)
			if !errors.Is(err, vectorstore.ErrInvalidBatcherOutput) {
				t.Fatalf("error = %v, want ErrInvalidBatcherOutput", err)
			}
		})
	}
}

func TestBatchDocumentsPreservesBatcherError(t *testing.T) {
	want := errors.New("batch failed")
	_, err := vectorstore.BatchDocuments(t.Context(), batcherFunc(func(
		context.Context,
		[]*document.Document,
	) ([][]*document.Document, error) {
		return nil, want
	}), nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
