package pinecone

import "errors"

// Sentinel errors for the [pinecone] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrNilConfig is returned when the validator receives nil.
	ErrNilConfig = errors.New("pinecone: config must not be nil")

	// ErrMissingClient is returned when the config supplies a nil
	// Pinecone client.
	ErrMissingClient = errors.New("pinecone: Client is required")

	// ErrMissingIndexHost is returned when IndexHost is empty.
	ErrMissingIndexHost = errors.New("pinecone: IndexHost is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("pinecone: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("pinecone: DocumentBatcher is required")
)
