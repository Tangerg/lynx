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
	// Add persists documents. Implementations return [ErrEmptyDocuments] when
	// docs is empty.
	Add(ctx context.Context, docs []*document.Document) error
}
