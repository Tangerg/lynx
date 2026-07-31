package vectorstore

import "errors"

// Sentinel errors for indexing and deletion preconditions. Search request
// validation returns descriptive value errors from [SearchRequest.Validate].
var (
	// ErrEmptyDocuments is returned by [Indexer.Add] on an empty slice.
	ErrEmptyDocuments = errors.New("vectorstore: documents must not be empty")

	// ErrInvalidDocument is returned by [Indexer.Add] when a document is nil,
	// malformed, or cannot be embedded by the text-oriented vector-store
	// contract.
	ErrInvalidDocument = errors.New("vectorstore: invalid document")

	// ErrMissingDocumentID identifies a document without a stable identity.
	// [Indexer.Add] requires caller-assigned IDs, and successful search results
	// must preserve the stored ID.
	ErrMissingDocumentID = errors.New("vectorstore: document ID is required")

	// ErrDuplicateDocumentID is returned by [Indexer.Add] when one call contains
	// the same ID more than once. Rejecting ambiguous batches keeps upsert order
	// independent of provider and batcher behavior.
	ErrDuplicateDocumentID = errors.New("vectorstore: duplicate document ID")

	// ErrMissingFilter is returned by [FilterDeleter.DeleteWhere] for nil.
	ErrMissingFilter = errors.New("vectorstore: filter is required")
)
