package qdrant

import "errors"

var (
	ErrMissingClient = errors.New("qdrant: Client is required")

	ErrMissingCollectionName = errors.New("qdrant: CollectionName is required")

	ErrMissingEmbeddingModel = errors.New("qdrant: EmbeddingModel is required")

	ErrMissingDocumentBatcher = errors.New("qdrant: DocumentBatcher is required")

	ErrInvalidDistanceMetric = errors.New("qdrant: DistanceMetric must be cosine, dot, euclid, or manhattan")

	ErrInvalidPointID = errors.New("qdrant: invalid point ID")

	ErrIncompatibleCollection = errors.New("qdrant: incompatible collection schema")
)
