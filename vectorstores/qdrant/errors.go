package qdrant

import "errors"

// Sentinel errors for the [qdrant] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrNilConfig is returned when the validator receives nil.
	ErrNilConfig = errors.New("qdrant: config must not be nil")

	// ErrMissingClient is returned when the config supplies a nil
	// Qdrant client.
	ErrMissingClient = errors.New("qdrant: Client is required")

	// ErrMissingCollectionName is returned when CollectionName is empty.
	ErrMissingCollectionName = errors.New("qdrant: CollectionName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("qdrant: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("qdrant: DocumentBatcher is required")
)
