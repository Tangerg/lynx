package vectorstore

import "errors"

// Sentinel errors for indexing and deletion preconditions. Search request
// validation returns descriptive value errors from [SearchRequest.Validate].
var (
	// ErrEmptyDocuments is returned by [Indexer.Add] on an empty slice.
	ErrEmptyDocuments = errors.New("vectorstore: documents must not be empty")

	// ErrMissingFilter is returned by [FilterDeleter.DeleteWhere] for nil.
	ErrMissingFilter = errors.New("vectorstore: filter is required")
)
