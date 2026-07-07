package vectorstore

import "errors"

// Sentinel errors for the request-shape validators. Callers can match
// these with [errors.Is] to distinguish "caller didn't fill the
// struct" from store-side failures.
var (
	// ErrNilRequest is returned by every Validate when the request
	// pointer is nil.
	ErrNilRequest = errors.New("vectorstore: request must not be nil")

	// ErrEmptyDocuments is returned by [CreateRequest.Validate] on an
	// empty document slice.
	ErrEmptyDocuments = errors.New("vectorstore: Documents must not be empty")

	// ErrMissingFilter is returned by [DeleteRequest.Validate] when no
	// filter expression has been supplied.
	ErrMissingFilter = errors.New("vectorstore: Filter is required")
)
