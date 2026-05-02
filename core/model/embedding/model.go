// Package embedding defines the request/response types and Model interface
// for text-to-vector embedding LLMs. Concrete provider implementations
// (OpenAI, Cohere, ...) live in /models/<provider>/embedding.go.
package embedding

import (
	"context"

	"github.com/Tangerg/lynx/core/model"
)

// Model is the provider surface for an embedding LLM — synchronous call,
// model defaults, dimensions probe, and identity hint.
//
// Example:
//
//	type myEmbedder struct{ /* ... */ }
//	func (m *myEmbedder) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) { ... }
//	func (m *myEmbedder) DefaultOptions() *embedding.Options { return embedding.NewOptionsOrPanic("text-embedding-3-small") }
//	func (m *myEmbedder) Info() embedding.ModelInfo          { return embedding.ModelInfo{Provider: "openai"} }
//	func (m *myEmbedder) Dimensions(ctx context.Context) int64 { return 1536 }
//
//	var _ embedding.Model = (*myEmbedder)(nil)
type Model interface {
	model.Model[*Request, *Response]

	// Dimensions returns the vector size this model produces (e.g. 768,
	// 1536). Implementations may probe the API on first call and cache the
	// result.
	Dimensions(ctx context.Context) int64

	// DefaultOptions returns the parameter set this provider uses when
	// the caller does not override anything.
	DefaultOptions() *Options

	// Info returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Info() ModelInfo
}

// ModelInfo holds identity metadata for a [Model] instance. Provider
// names are conventionally lowercase ("openai", "cohere", ...).
type ModelInfo struct {
	// Provider names the embedding LLM vendor.
	Provider string `json:"provider"`
}
