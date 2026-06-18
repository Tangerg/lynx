package tts

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for a text-to-speech LLM. It supports
// both synchronous full-audio generation and streaming chunked output.
//
// Example:
//
//	type myTTS struct{ /* ... */ }
//	func (m *myTTS) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) { ... }
//	func (m *myTTS) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] { ... }
//	func (m *myTTS) DefaultOptions() tts.Options { ... }
//	func (m *myTTS) Metadata() tts.ModelMetadata          { return tts.ModelMetadata{Provider: "openai"} }
//
//	var _ tts.Model = (*myTTS)(nil)
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() Options

	// Metadata returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Metadata() ModelMetadata
}

// ModelMetadata holds identity metadata for a [Model] instance.
type ModelMetadata struct {
	Provider string `json:"provider"`
}
