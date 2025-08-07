package document

import (
	"context"
)

// Reader defines an interface for reading documents from a source.
// Implementations can retrieve documents from various data sources such as
// databases, file systems, or external APIs.
type Reader interface {
	// Read retrieves documents from the underlying source.
	// Returns a slice of Document objects or an error if the operation fails.
	Read(ctx context.Context) ([]*Document, error)
}

// EmptyReader is a no-operation reader that returns no documents.
// It implements the Reader interface but always returns an empty slice.
type EmptyReader struct{}

func NewEmptyReader() *EmptyReader {
	return &EmptyReader{}
}

// Read returns an empty document slice and no error.
// This is useful for testing or when no data source is available.
func (e *EmptyReader) Read(_ context.Context) ([]*Document, error) {
	return []*Document{}, nil
}
