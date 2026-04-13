package embedding

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/pkg/assert"
)

// dimensionsStore is a thread-safe cache that stores the embedding dimensions
// for each model. The key is the full model identifier (provider:model),
// and the value is the dimension count as int64.
var dimensionsStore sync.Map

// GetDimensions retrieves the embedding dimension size for a given model.
// It uses a cache to avoid redundant API calls for the same model.
//
// Parameters:
//   - ctx: The context for handling cancellation and timeouts
//   - model: The embedding model to query
//
// Returns:
//   - int64: The dimension size of the embedding vectors produced by the model.
//     Returns 0 if an error occurs during the embedding call.
//
// The function first checks the cache (dimensionsStore) for the model's dimensions.
// If not found, it makes a test embedding call with the text "test" to determine
// the dimension size, then caches the result for future use.
func GetDimensions(ctx context.Context, model Model) int64 {
	fullModel := model.Info().Provider + ":" + model.DefaultOptions().Model

	value, ok := dimensionsStore.Load(fullModel)
	if ok {
		return value.(int64)
	}

	resp, err := model.Call(ctx, assert.Must(NewRequest([]string{"test"})))
	if err != nil {
		return 0
	}

	dimensions := int64(len(resp.Result().Embedding))

	dimensionsStore.Store(fullModel, dimensions)

	return dimensions
}
