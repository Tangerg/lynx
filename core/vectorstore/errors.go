package vectorstore

import "errors"

// Sentinel errors keep request/options/response handling consistent with the
// other core model packages while retaining vector-store-specific causes.
var (
	// ErrInvalidOptions reports malformed search options.
	ErrInvalidOptions = errors.New("vectorstore: invalid options")

	// ErrInvalidRequest reports a malformed indexing or search request.
	ErrInvalidRequest = errors.New("vectorstore: invalid request")

	// ErrInvalidResponse reports malformed search result data.
	ErrInvalidResponse = errors.New("vectorstore: invalid response")

	// ErrInvalidScore reports a score outside the provider-neutral contract.
	ErrInvalidScore = errors.New("vectorstore: invalid score")

	// ErrEmptyDocuments is returned by [IndexRequest.Validate] for an empty request.
	ErrEmptyDocuments = errors.New("vectorstore: documents must not be empty")

	// ErrInvalidDocument is returned by [IndexRequest.Validate] when a document is nil,
	// malformed, or cannot be embedded by the text-oriented vector-store
	// contract.
	ErrInvalidDocument = errors.New("vectorstore: invalid document")

	// ErrMissingDocumentID identifies a document without a stable identity.
	// [Indexer.Index] requires caller-assigned IDs, and successful search results
	// must preserve the stored ID.
	ErrMissingDocumentID = errors.New("vectorstore: document ID is required")

	// ErrDuplicateDocumentID is returned by [Indexer.Index] when one call contains
	// the same ID more than once. Rejecting ambiguous batches keeps upsert order
	// independent of provider and batcher behavior.
	ErrDuplicateDocumentID = errors.New("vectorstore: duplicate document ID")

	// ErrMissingFilter is returned by [FilterDeleter.DeleteWhere] for nil.
	ErrMissingFilter = errors.New("vectorstore: filter is required")
)

// ErrInvalidBatcherOutput identifies a [Batcher] result that is not an
// order-preserving partition of its input.
var ErrInvalidBatcherOutput = errors.New("vectorstore: invalid batcher output")
