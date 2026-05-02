// Package image defines the request/response types and Model interface
// for image-generation LLMs. Concrete provider implementations
// (OpenAI DALL·E, Stability AI, ...) live in /models/<provider>/image.go.
package image

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for an image-generation LLM —
// synchronous call, model defaults, and an identity hint.
//
// Example:
//
//	type myImageModel struct{ /* ... */ }
//	func (m *myImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) { ... }
//	func (m *myImageModel) DefaultOptions() *image.Options { return image.NewOptionsOrPanic("dall-e-3") }
//	func (m *myImageModel) Info() image.ModelInfo         { return image.ModelInfo{Provider: "openai"} }
//
//	var _ image.Model = (*myImageModel)(nil)
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() *Options

	// Info returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Info() ModelInfo
}

// ModelInfo holds identity metadata for a [Model] instance. Provider
// names are conventionally lowercase ("openai", "stability", ...).
type ModelInfo struct {
	// Provider names the image-generation LLM vendor.
	Provider string `json:"provider"`
}
