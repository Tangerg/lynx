package pinecone

import "errors"

var (
	ErrMissingClient = errors.New("pinecone: Client is required")

	ErrMissingIndexHost = errors.New("pinecone: IndexHost is required")

	ErrMissingEmbeddingModel = errors.New("pinecone: EmbeddingModel is required")

	ErrMissingDocumentBatcher = errors.New("pinecone: DocumentBatcher is required")

	ErrMissingDistanceMetric = errors.New("pinecone: DistanceMetric is required")
)
