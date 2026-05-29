package chat

import (
	"github.com/Tangerg/lynx/core/model"
)

// Model is the provider surface: a chat LLM that supports both synchronous
// [model.Model.Call] and streaming [model.StreamingModel.Stream]. Implementations
// expose model-specific defaults via [Model.DefaultOptions] and an identity
// hint via [Model.Metadata] so callers and observability layers can branch on
// provider.
//
// Example:
//
//	type myModel struct{ /* ... */ }
//	func (m *myModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) { ... }
//	func (m *myModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] { ... }
//	func (m *myModel) DefaultOptions() chat.Options { opts, _ := chat.NewOptions("gpt-4o"); return *opts }
//	func (m *myModel) Metadata() chat.ModelMetadata          { return chat.ModelMetadata{Provider: "openai"} }
//
//	var _ chat.Model = (*myModel)(nil)
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when the
	// caller does not override anything. The returned value is a fresh copy
	// the caller may mutate.
	DefaultOptions() Options

	// Metadata returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Metadata() ModelMetadata
}
