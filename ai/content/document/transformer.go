package document

import (
	"context"
)

// Transformer defines an interface for processing and transforming documents.
// Implementations can modify, filter, enrich, or otherwise transform documents
// as they pass through a processing pipeline.
type Transformer interface {
	// Transform processes a batch of documents and returns the transformed result.
	// Returns transformed Document objects or an error if the operation fails.
	Transform(ctx context.Context, docs []*Document) ([]*Document, error)
}

// NopTransformer is a no-operation transformer that passes documents through unchanged.
// It implements the Transformer interface without applying any transformations.
type NopTransformer struct{}

func NewNopTransformer() *NopTransformer {
	return &NopTransformer{}
}

// Transform returns the input documents without any modifications.
// This is useful for testing or as a placeholder in processing pipelines.
func (n *NopTransformer) Transform(_ context.Context, docs []*Document) ([]*Document, error) {
	return docs, nil
}
