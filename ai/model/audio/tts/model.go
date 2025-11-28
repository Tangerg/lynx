package tts

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Model represents a text-to-speech model interface that combines base model functionality
// with streaming capabilities for TTS operations. It processes speech generation requests
// and returns audio responses, supporting both synchronous and streaming modes.
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns the default configuration options for this TTS model
	DefaultOptions() *Options

	// Info returns metadata information about this TTS model
	Info() ModelInfo
}

// ModelInfo contains metadata information about a text-to-speech model
type ModelInfo struct {
	// Provider identifies the service or organization that provides this TTS model
	// Examples: "OpenAI", "ElevenLabs", etc.
	Provider string `json:"provider"`
}
