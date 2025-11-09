package embedding

import (
	"context"

	"github.com/Tangerg/lynx/ai/model"
)

// Model defines the interface for embedding models that can convert text into vector representations.
// It extends the base model.Model interface and adds embedding-specific capabilities.
type Model interface {
	model.Model[*Request, *Response]

	// Dimensions returns the dimensionality of the embedding vectors produced by this model.
	// For example, a model might return 768 or 1536 dimensional vectors.
	Dimensions(ctx context.Context) int64

	// DefaultOptions returns the default configuration options for this embedding model.
	// These options can be overridden when making embedding requests.
	DefaultOptions() *Options

	// Info returns metadata information about the embedding model,
	// such as the provider name and other model characteristics.
	Info() ModelInfo
}

// ModelInfo contains metadata information about an embedding model.
type ModelInfo struct {
	// Provider identifies the service or organization that provides this embedding model.
	// Examples: "OpenAI", "Cohere", "HuggingFace", etc.
	Provider string `json:"provider"`
}
