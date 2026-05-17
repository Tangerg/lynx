package embedding

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/pkg/assert"
)

// dimensionsStore caches the dimension count per "provider:model" key
// so [GetDimensions] doesn't need to round-trip the provider on every
// call.
var dimensionsStore sync.Map

// GetDimensions reports how many components an embedding from the given
// model has. The first call probes the provider with a one-token "test"
// input and caches the answer; subsequent calls are O(1).
//
// Returns 0 when the probe call fails — callers should treat 0 as
// "unknown" rather than a real dimension count.
//
// Example:
//
//	d := embedding.GetDimensions(ctx, openaiEmbedding) // → 1536
func GetDimensions(ctx context.Context, model Model) int64 {
	cacheKey := model.Metadata().Provider + ":" + model.DefaultOptions().Model

	if value, ok := dimensionsStore.Load(cacheKey); ok {
		return value.(int64)
	}

	resp, err := model.Call(ctx, assert.Must(NewRequest([]string{"test"})))
	if err != nil {
		return 0
	}

	dimensions := int64(len(resp.Result().Embedding))
	dimensionsStore.Store(cacheKey, dimensions)
	return dimensions
}
