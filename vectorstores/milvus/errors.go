package milvus

import "errors"

var (
	ErrMissingClient = errors.New("milvus: Client is required")

	ErrMissingCollectionName = errors.New("milvus: CollectionName is required")

	ErrMissingEmbeddingModel = errors.New("milvus: EmbeddingModel is required")

	ErrMissingDocumentBatcher = errors.New("milvus: DocumentBatcher is required")

	ErrDocumentIDTooLong = errors.New("milvus: document ID exceeds the 36-byte limit")

	ErrDocumentContentTooLong = errors.New("milvus: document text exceeds the 65535-byte limit")
)
