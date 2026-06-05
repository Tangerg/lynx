package document

import "context"

// Reader sources documents — from files, databases, APIs, or any other
// origin. Concrete readers live alongside this interface
// ([reader_text.go], [reader_json.go]).
type Reader interface {
	// Read returns the documents discovered at the underlying source.
	Read(ctx context.Context) ([]*Document, error)
}

// Writer persists documents — to files, databases, vector stores, or
// any other sink. The error contract is "all-or-nothing on best
// effort" — implementations document their transactionality.
type Writer interface {
	// Write stores docs at the underlying destination. An error from a
	// later document may or may not roll back earlier writes; consult
	// the implementation's docs.
	Write(ctx context.Context, docs []*Document) error
}

// MetadataMode selects how much metadata a [Formatter] embeds in its
// output — full dump for debugging, embedding-friendly subset for
// vector stores, inference-time-only fields for prompts, or none when
// the consumer cares only about the body.
type MetadataMode string

func (m MetadataMode) String() string { return string(m) }

const (
	// MetadataModeAll includes every metadata key.
	MetadataModeAll MetadataMode = "all"

	// MetadataModeEmbed includes only metadata appropriate for vector
	// embedding generation.
	MetadataModeEmbed MetadataMode = "embed"

	// MetadataModeInference includes only metadata that should reach
	// the model at prompt time.
	MetadataModeInference MetadataMode = "inference"

	// MetadataModeNone strips every metadata key — body only.
	MetadataModeNone MetadataMode = "none"
)

// Formatter renders a document as a string. Implementations must
// handle nil documents gracefully and respect the supplied mode.
type Formatter interface {
	// Format renders doc honoring the metadata mode.
	Format(doc *Document, mode MetadataMode) string
}

// Transformer is one stage in a document-processing pipeline —
// splitting, filtering, enriching, deduplicating, etc. The output
// length may differ from the input length.
type Transformer interface {
	// Transform processes docs and returns the transformed slice.
	Transform(ctx context.Context, docs []*Document) ([]*Document, error)
}

// Batcher carves a document slice into chunks that fit downstream
// service constraints (token limits, request size). Document order MUST
// be preserved across batches so callers can map results back by index.
type Batcher interface {
	// Batch returns sub-slices of docs sized for downstream consumers.
	// Concatenating the returned batches reproduces docs.
	Batch(ctx context.Context, docs []*Document) ([][]*Document, error)
}
