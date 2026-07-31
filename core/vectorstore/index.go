package vectorstore

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// Indexer embeds and indexes documents in the vector store. The store
// runs:
//
//  1. Embedding (text → vector)
//  2. Indexing (vector + metadata → searchable record)
//  3. Storage (record → durable backend)
type Indexer interface {
	// Add persists documents using caller-assigned IDs. Existing IDs are
	// replaced according to the backend's upsert semantics. Implementations
	// validate the complete input before external I/O and return:
	//   - [ErrEmptyDocuments] when docs is empty,
	//   - [ErrInvalidDocument] when a document is nil or malformed,
	//   - [ErrMissingDocumentID] when an ID is empty, and
	//   - [ErrDuplicateDocumentID] when an ID occurs more than once.
	//
	// Add never invents document IDs: its error-only result has no channel for
	// returning generated identities to the caller.
	Add(ctx context.Context, docs []*document.Document) error
}
