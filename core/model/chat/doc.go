// Package chat provides the request/response types and the Model interface
// for conversational LLMs. Concrete provider implementations (OpenAI,
// Anthropic, Google, ...) live in /models/<provider>/chat.go.
//
// The package layers stay parallel to the rest of /core/model:
//
//	Model           — the provider surface (Call + Stream + DefaultOptions + Metadata)
//	Request/Response— the in/out value types
//	Message         — sealed message hierarchy (system / user / assistant / tool)
//	Tool            — function/tool definitions and registry
//	Client          — the fluent caller wrapping Model
//	memory          — multi-turn message stores and middleware
package chat
