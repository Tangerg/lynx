package weaviate

import "errors"

// Sentinel errors for the [weaviate] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrNilConfig is returned when the validator receives nil.
	ErrNilConfig = errors.New("weaviate: config must not be nil")

	// ErrMissingClient is returned when the config supplies a nil
	// Weaviate client.
	ErrMissingClient = errors.New("weaviate: Client is required")

	// ErrMissingClassName is returned when ClassName is empty.
	ErrMissingClassName = errors.New("weaviate: ClassName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("weaviate: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("weaviate: DocumentBatcher is required")
)
