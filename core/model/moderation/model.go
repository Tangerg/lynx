// Package moderation defines the request/response types and Model
// interface for content-moderation LLMs. Concrete provider implementations
// (OpenAI moderation, Mistral, ...) live in /models/<provider>/moderation.go.
package moderation

import "github.com/Tangerg/lynx/core/model"

// Model is the provider surface for a moderation LLM — synchronous
// call, model defaults, identity hint.
//
// Example:
//
//	type myModerator struct{ /* ... */ }
//	func (m *myModerator) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) { ... }
//	func (m *myModerator) DefaultOptions() *moderation.Options { ... }
//	func (m *myModerator) Info() moderation.ModelInfo          { return moderation.ModelInfo{Provider: "openai"} }
//
//	var _ moderation.Model = (*myModerator)(nil)
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
	// Provider names the moderation LLM vendor (lowercase by convention).
	Provider string `json:"provider"`
}
