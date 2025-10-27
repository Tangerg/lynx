package document

import (
	"context"
)

// Reader defines an interface for reading documents from various data sources.
// Implementations can retrieve documents from databases, file systems, APIs,
// or other storage systems with proper error handling and context support.
type Reader interface {
	// Read retrieves documents from the underlying source with context support.
	// Returns a slice of Document objects or an error if the operation fails.
	// Context can be used for timeouts, cancellation, and request-scoped values.
	Read(ctx context.Context) ([]*Document, error)
}

// Writer defines an interface for writing documents to various destinations.
// Implementations can store documents in databases, file systems, APIs,
// or other storage systems with transactional support where applicable.
type Writer interface {
	// Write stores documents in the underlying destination with context support.
	// Returns an error if any document fails to write, potentially rolling back
	// the entire operation depending on the implementation's transaction behavior.
	Write(ctx context.Context, docs []*Document) error
}

// MetadataMode defines how metadata should be handled when formatting documents.
// Different modes control which metadata fields are included in the output,
// allowing optimization for specific use cases like embedding or inference.
type MetadataMode string

const (
	// MetadataModeAll includes all available metadata in the formatted content.
	// Use this mode when you need complete document information.
	MetadataModeAll MetadataMode = "all"

	// MetadataModeEmbed includes only metadata relevant for embedding processes.
	// This mode optimizes content for vector embedding generation.
	MetadataModeEmbed MetadataMode = "embed"

	// MetadataModeInference includes only metadata relevant for inference operations.
	// This mode focuses on metadata that affects model inference behavior.
	MetadataModeInference MetadataMode = "inference"

	// MetadataModeNone excludes all metadata from the formatted content.
	// Use this mode when you only need the raw document content.
	MetadataModeNone MetadataMode = "none"
)

// Formatter defines an interface for formatting document content with flexible
// metadata inclusion. Implementations should handle various document types
// and provide consistent output formatting across different metadata modes.
type Formatter interface {
	// Format produces a string representation of a document with controlled metadata.
	// The mode parameter determines which metadata fields are included in the output.
	// Implementations should handle nil documents gracefully and provide meaningful defaults.
	Format(doc *Document, mode MetadataMode) string
}

// Transformer defines an interface for transforming documents in processing pipelines.
// Implementations can modify, filter, enrich, or validate documents while maintaining
// proper error handling and context support for cancellation and timeouts.
type Transformer interface {
	// Transform handles a batch of documents and returns the transformed result.
	// The returned slice may have different length than input (filtering/expansion).
	// Context should be respected for cancellation and timeout handling.
	Transform(ctx context.Context, docs []*Document) ([]*Document, error)
}

// Batcher defines an interface for optimizing document batching for embedding operations.
// Implementations should consider token limits, memory constraints, and processing
// efficiency while preserving document order for correct embedding-to-document mapping.
type Batcher interface {
	// Batch splits documents into optimized sub-batches for embedding processing.
	// Document order must be preserved across all batches for correct mapping.
	// Returns batches optimized for the target embedding service's constraints.
	Batch(ctx context.Context, docs []*Document) ([][]*Document, error)
}
