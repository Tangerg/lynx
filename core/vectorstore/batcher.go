package vectorstore

import (
	"context"

	"github.com/Tangerg/scope/core/document"
)

// Batcher partitions documents for ingestion. It must preserve every document
// pointer exactly once and in input order, and it must not return empty
// batches. Implementations commonly come from document pipelines; stores
// depend only on this narrow capability contract.
type Batcher interface {
	Batch(context.Context, []*document.Document) ([][]*document.Document, error)
}
