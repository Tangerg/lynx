package moderation

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for a moderation LLM — synchronous
// call, model defaults, identity hint.
//
// Example:
//
//	type myModerator struct{ /* ... */ }
//	func (m *myModerator) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) { ... }
//	func (m *myModerator) DefaultOptions() moderation.Options { ... }
//	func (m *myModerator) Metadata() moderation.ModelMetadata          { return moderation.ModelMetadata{Provider: "openai"} }
//
//	var _ moderation.Model = (*myModerator)(nil)
type Model interface {
	model.Model[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() Options

	// Metadata returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Metadata() ModelMetadata
}

// ModelMetadata holds identity metadata for a [Model] instance.
type ModelMetadata struct {
	// Provider names the moderation LLM vendor (lowercase by convention).
	Provider string `json:"provider"`
}
