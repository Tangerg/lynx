package chroma

import "errors"

var (
	ErrMissingClient = errors.New("chroma: Client is required")

	ErrMissingCollectionName = errors.New("chroma: CollectionName is required")

	ErrMissingEmbeddingModel = errors.New("chroma: EmbeddingModel is required")

	ErrMissingDocumentBatcher = errors.New("chroma: DocumentBatcher is required")
)
