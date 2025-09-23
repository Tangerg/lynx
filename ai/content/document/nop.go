package document

import (
	"context"
)

var _ Reader = (*Nop)(nil)
var _ Writer = (*Nop)(nil)
var _ Processor = (*Nop)(nil)
var _ Formatter = (*Nop)(nil)
var _ Batcher = (*Nop)(nil)

// Nop provides no-operation implementations for all document interfaces.
// Used as default behavior, testing mock, or placeholder in pipeline configurations
// where certain operations should be skipped without causing errors.
type Nop struct{}

// NewNop creates a new no-operation implementation instance.
// Useful for initializing default behaviors or testing scenarios.
func NewNop() *Nop {
	return &Nop{}
}

// Read returns an empty document slice without performing any actual reading.
// Satisfies the Reader interface for cases where no input is needed.
func (n *Nop) Read(_ context.Context) ([]*Document, error) {
	return []*Document{}, nil
}

// Write accepts documents but performs no actual storage operation.
// Always succeeds, making it safe for testing and default configurations.
func (n *Nop) Write(_ context.Context, _ []*Document) error {
	return nil
}

// Format returns only the document's text content, ignoring metadata mode.
// Provides minimal formatting suitable for basic text extraction scenarios.
func (n *Nop) Format(doc *Document, _ MetadataMode) string {
	return doc.Text()
}

// Process returns documents unchanged without any transformation.
// Useful as a pass-through processor in pipeline configurations.
func (n *Nop) Process(_ context.Context, docs []*Document) ([]*Document, error) {
	return docs, nil
}

// Batch returns all documents in a single batch without optimization.
// Suitable for scenarios where batching logic should be bypassed.
func (n *Nop) Batch(_ context.Context, docs []*Document) ([][]*Document, error) {
	return [][]*Document{docs}, nil
}
