package image

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Model represents an image generation model interface that combines base model functionality
// with image-specific operations. It processes image generation requests and returns responses.
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the default configuration options for this image model
	DefaultOptions() *Options

	// Info returns metadata information about this image model
	Info() ModelInfo
}

// ModelInfo contains metadata information about an image model.
type ModelInfo struct {
	// Provider identifies the service or organization that provides this image model.
	// Examples: "OpenAI", "Stability AI", "Midjourney", etc.
	Provider string `json:"provider"`
}
