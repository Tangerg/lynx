package image

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for an image-generation LLM —
// synchronous call, model defaults, and an identity hint.
//
// Example:
//
//	type myImageModel struct{ /* ... */ }
//	func (m *myImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) { ... }
//	func (m *myImageModel) DefaultOptions() image.Options { opts, _ := image.NewOptions("dall-e-3"); return *opts }
//	func (m *myImageModel) Metadata() image.ModelMetadata         { return image.ModelMetadata{Provider: "openai"} }
//
//	var _ image.Model = (*myImageModel)(nil)
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() Options

	// Metadata returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Metadata() ModelMetadata
}

// ModelMetadata holds identity metadata for a [Model] instance. Provider
// names are conventionally lowercase ("openai", "stability", ...).
type ModelMetadata struct {
	Provider string `json:"provider"`
}
