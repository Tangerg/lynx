// Package chat provides the request/response types and the Model interface
// for conversational LLMs. Concrete provider implementations (OpenAI,
// Anthropic, Google, ...) live in /models/<provider>/chat.go.
//
// The package layers stay parallel to the rest of /core/model:
//
//	Model           — the provider surface (Call + Stream + DefaultOptions + Info)
//	Request/Response— the in/out value types
//	Message         — sealed message hierarchy (system / user / assistant / tool)
//	Tool            — function/tool definitions and registry
//	Client          — the fluent caller wrapping Model
//	memory          — multi-turn message stores and middleware
package chat

import (
	"github.com/Tangerg/lynx/core/model"
)

// Model is the provider surface: a chat LLM that supports both synchronous
// [model.Model.Call] and streaming [model.StreamingModel.Stream]. Implementations
// expose model-specific defaults via [Model.DefaultOptions] and an identity
// hint via [Model.Info] so callers and observability layers can branch on
// provider.
//
// Example:
//
//	type myModel struct{ /* ... */ }
//	func (m *myModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) { ... }
//	func (m *myModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] { ... }
//	func (m *myModel) DefaultOptions() *chat.Options { return chat.NewOptionsOrPanic("gpt-4o") }
//	func (m *myModel) Info() chat.ModelInfo          { return chat.ModelInfo{Provider: "openai"} }
//
//	var _ chat.Model = (*myModel)(nil)
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when the
	// caller does not override anything. The returned value is a fresh copy
	// the caller may mutate.
	DefaultOptions() *Options

	// Info returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Info() ModelInfo
}

// ModelInfo holds identity metadata for a [Model] instance. Provider names
// are conventionally lowercase (e.g. "openai", "anthropic", "google") so
// downstream filters can match without case folding.
type ModelInfo struct {
	// Provider names the LLM vendor — "openai", "anthropic", "google", etc.
	Provider string `json:"provider"`
}
