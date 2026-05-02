// Package transcription defines the request/response types and Model
// interface for audio-to-text LLMs. Concrete provider implementations
// (OpenAI Whisper, Deepgram, AssemblyAI, ...) live in
// /models/<provider>/audio_transcription.go.
package transcription

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for an audio transcription LLM —
// synchronous call, model defaults, identity hint.
//
// Example:
//
//	type myWhisper struct{ /* ... */ }
//	func (m *myWhisper) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) { ... }
//	func (m *myWhisper) DefaultOptions() *transcription.Options { ... }
//	func (m *myWhisper) Info() transcription.ModelInfo          { return transcription.ModelInfo{Provider: "openai"} }
//
//	var _ transcription.Model = (*myWhisper)(nil)
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() *Options

	// Info returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Info() ModelInfo
}

// ModelInfo holds identity metadata for a [Model] instance.
type ModelInfo struct {
	// Provider names the transcription LLM vendor (lowercase by convention).
	Provider string `json:"provider"`
}
