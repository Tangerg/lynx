package embedding

import (
	"context"
	"github.com/Tangerg/lynx/ai/commons/document"
)

// BatchingStrategy is a contract for batching Document objects so that the call to embed them could be
// optimized.
type BatchingStrategy interface {
	// Batch optimizes embedding tokens by splitting the incoming collection of Documents into sub-batches.
	// EmbeddingModel implementations can call this method to optimize embedding tokens.
	// It is important to preserve the order of the list of Documents when batching as
	// they are mapped to their corresponding embeddings by their order.
	//
	// Parameters:
	//   ctx - the context for cancellation and timeout control
	//   docs - documents to batch
	//
	// Returns:
	//   A list of sub-batches that contain Documents, or an error if batching fails.
	Batch(ctx context.Context, docs []*document.Document) ([][]*document.Document, error)
}
