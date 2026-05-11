package chroma

import "errors"

// Sentinel errors for the [chroma] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrNilConfig is returned when the validator receives nil.
	ErrNilConfig = errors.New("chroma: config must not be nil")

	// ErrMissingClient is returned when the config supplies a nil
	// Chroma HTTP client.
	ErrMissingClient = errors.New("chroma: Client is required")

	// ErrMissingCollectionName is returned when CollectionName is empty.
	ErrMissingCollectionName = errors.New("chroma: CollectionName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("chroma: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("chroma: DocumentBatcher is required")
)
