package vectorstores

import (
	"context"

	"github.com/Tangerg/lynx/core/document"
)

// Batcher is the only ingestion batching capability required by vector store
// adapters. Implementations commonly come from the documentpipeline module,
// but adapters depend on this local contract instead of that framework.
type Batcher interface {
	Batch(context.Context, []*document.Document) ([][]*document.Document, error)
}
