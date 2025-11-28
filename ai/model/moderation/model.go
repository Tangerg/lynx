package moderation

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Model represents a content moderation model interface that combines base model functionality
// with moderation-specific operations. It processes moderation requests and returns safety analysis responses.
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the default configuration options for this moderation model
	DefaultOptions() *Options

	// Info returns metadata information about this moderation model
	Info() ModelInfo
}

// ModelInfo contains metadata information about a moderation model
type ModelInfo struct {
	// Provider identifies the service or organization that provides this moderation model
	// Examples: "OpenAI", etc.
	Provider string `json:"provider"`
}
