package qdrant

import "errors"

// Sentinel errors for the [qdrant] package's input-shape validators.
// Callers can match these with [errors.Is] to distinguish caller-side
// input errors from transport, SDK, or backend failures.
var (
	// ErrMissingClient is returned when the config supplies a nil
	// Qdrant client.
	ErrMissingClient = errors.New("qdrant: Client is required")

	// ErrMissingCollectionName is returned when CollectionName is empty.
	ErrMissingCollectionName = errors.New("qdrant: CollectionName is required")

	// ErrMissingEmbeddingModel is returned when EmbeddingModel is nil.
	ErrMissingEmbeddingModel = errors.New("qdrant: EmbeddingModel is required")

	// ErrMissingDocumentBatcher is returned when DocumentBatcher is nil.
	ErrMissingDocumentBatcher = errors.New("qdrant: DocumentBatcher is required")

	// ErrInvalidDistanceMetric is returned when DistanceMetric is not one of
	// the metrics supported by Qdrant dense-vector collections.
	ErrInvalidDistanceMetric = errors.New("qdrant: DistanceMetric must be cosine, dot, euclid, or manhattan")

	// ErrInvalidPointID is returned when an ID is neither a canonical uint64
	// nor a UUID, the two identity shapes accepted by Qdrant.
	ErrInvalidPointID = errors.New("qdrant: invalid point ID")

	// ErrIncompatibleCollection is returned during explicit schema
	// initialization when an existing collection cannot satisfy this store's
	// unnamed dense-vector contract.
	ErrIncompatibleCollection = errors.New("qdrant: incompatible collection schema")
)
