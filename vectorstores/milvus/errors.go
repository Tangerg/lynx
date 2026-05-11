package milvus

import "errors"

// Sentinel errors for the [milvus] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrNilConfig is returned when the validator receives nil.
	ErrNilConfig = errors.New("milvus: config must not be nil")

	// ErrMissingClient is returned when the config supplies a nil
	// Milvus client.
	ErrMissingClient = errors.New("milvus: Client is required")

	// ErrMissingCollectionName is returned when CollectionName is empty.
	ErrMissingCollectionName = errors.New("milvus: CollectionName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("milvus: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("milvus: DocumentBatcher is required")
)
