package vectorstores

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// Batcher partitions documents for ingestion. It must preserve every document
// pointer exactly once and in input order, and it must not return empty
// batches. Implementations commonly come from the documentpipeline module,
// but adapters depend on this local contract instead of that framework.
type Batcher interface {
	Batch(context.Context, []*document.Document) ([][]*document.Document, error)
}
