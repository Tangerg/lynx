package document

import (
	"context"
)

// Writer defines an interface for writing documents to a destination.
// Implementations can store documents in various data stores such as
// databases, file systems, or external services.
type Writer interface {
	// Write stores documents in the underlying destination.
	// Returns an error if the operation fails.
	Write(ctx context.Context, docs []*Document) error
}

// DiscardWriter is a no-operation writer that discards all documents without storing them.
// It implements the Writer interface but performs no actual write operations.
type DiscardWriter struct{}

func NewDiscardWriter() *DiscardWriter {
	return &DiscardWriter{}
}

// Write discards the input documents and always returns nil.
// This is useful for testing or when write operations need to be disabled.
func (d *DiscardWriter) Write(_ context.Context, _ []*Document) error {
	return nil
}
