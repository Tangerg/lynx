package milvus

import "errors"

// Sentinel errors for the [milvus] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrMissingClient is returned when the config supplies a nil
	// Milvus client.
	ErrMissingClient = errors.New("milvus: Client is required")

	// ErrMissingCollectionName is returned when CollectionName is empty.
	ErrMissingCollectionName = errors.New("milvus: CollectionName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("milvus: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("milvus: DocumentBatcher is required")

	// ErrDocumentIDTooLong is returned when an ID cannot fit the collection's
	// primary-key VarChar field.
	ErrDocumentIDTooLong = errors.New("milvus: document ID exceeds the 36-byte limit")

	// ErrDocumentContentTooLong is returned when text cannot fit the
	// collection's content VarChar field.
	ErrDocumentContentTooLong = errors.New("milvus: document text exceeds the 65535-byte limit")
)
