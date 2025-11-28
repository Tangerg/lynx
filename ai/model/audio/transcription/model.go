package transcription

import (
	"github.com/Tangerg/lynx/ai/model"
)

// Model represents an audio transcription model interface that processes audio-to-text
// conversion requests and returns transcription responses. It supports synchronous
// transcription operations for converting speech or audio into written text.
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the default configuration options for this transcription model
	DefaultOptions() *Options

	// Info returns metadata information about this transcription model
	Info() ModelInfo
}

// ModelInfo contains metadata information about an audio transcription model
type ModelInfo struct {
	// Provider identifies the service or organization that provides this transcription model
	// Examples: "OpenAI", "Google", "Azure", "AssemblyAI", "Deepgram", etc.
	Provider string `json:"provider"`
}
