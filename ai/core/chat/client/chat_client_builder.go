package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

// ChatClientBuilder is a generic interface that defines the contract for building and configuring
// a chat client in a chat application. It is parameterized by chat options (O) and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// DefaultChatOptions(options O) ChatClientBuilder[O, M]
//   - Sets the default chat options for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultMiddlewares(middlewares Middlewares[O, M]) ChatClientBuilder[O, M]
//   - Sets the default middleware functions for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultMiddlewaresWithParams(params map[string]any, middlewares ...middleware.Middleware[O, M]) ChatClientBuilder[O, M]
//   - Sets the default middleware functions with additional parameters for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultUserPromptText(text string) ChatClientBuilder[O, M]
//   - Sets the default user prompt text for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
//   - Sets the default user prompt text with additional parameters for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M]
//   - Sets the default user prompt using a UserPrompt object for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultSystemPromptText(text string) ChatClientBuilder[O, M]
//   - Sets the default system prompt text for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
//   - Sets the default system prompt text with additional parameters for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M]
//   - Sets the default system prompt using a SystemPrompt object for the chat client.
//   - Returns the ChatClientBuilder instance to allow method chaining.
//
// Build() ChatClient[O, M]
//   - Finalizes the configuration and builds the chat client.
//   - Returns a ChatClient instance configured with the specified defaults.
type ChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	DefaultChatOptions(options O) ChatClientBuilder[O, M]
	DefaultMiddlewares(middlewares Middlewares[O, M]) ChatClientBuilder[O, M]
	DefaultMiddlewaresWithParams(params map[string]any, middlewares ...middleware.Middleware[O, M]) ChatClientBuilder[O, M]
	DefaultUserPromptText(text string) ChatClientBuilder[O, M]
	DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
	DefaultUserPromptTextWihtParamsAndMedia(text string, params map[string]any, media ...*media.Media) ChatClientBuilder[O, M]
	DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M]
	DefaultSystemPromptText(text string) ChatClientBuilder[O, M]
	DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
	DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M]
	Build() ChatClient[O, M]
}

func NewDefaultChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](chatModel model.ChatModel[O, M]) *DefaultChatClientBuilder[O, M] {

	request, _ := NewDefaultChatClientRequestBuilder[O, M]().
		WithChatModel(chatModel).
		Build()

	return &DefaultChatClientBuilder[O, M]{
		request: request,
	}
}

var _ ChatClientBuilder[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultChatClientBuilder[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func (d *DefaultChatClientBuilder[O, M]) DefaultMiddlewaresWithParams(params map[string]any, middlewares ...middleware.Middleware[O, M]) ChatClientBuilder[O, M] {
	return d.DefaultMiddlewares(
		NewDefaultMiddlewares[O, M]().
			SetMiddlewares(middlewares...).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultMiddlewares(middlewares Middlewares[O, M]) ChatClientBuilder[O, M] {
	d.request.SetMiddlewares(middlewares)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) DefaultChatOptions(options O) ChatClientBuilder[O, M] {
	d.request.SetChatOptions(options)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptText(text string) ChatClientBuilder[O, M] {
	return d.DefaultUserPromptTextWihtParams(text, nil)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M] {
	return d.DefaultUserPromptTextWihtParamsAndMedia(text, params)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptTextWihtParamsAndMedia(text string, params map[string]any, media ...*media.Media) ChatClientBuilder[O, M] {
	return d.DefaultUserPrompt(
		NewDefaultUserPrompt().
			SetText(text).
			SetParams(params).
			SetMedia(media...),
	)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M] {
	d.request.SetUserPrompt(user)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) DefaultSystemPromptText(text string) ChatClientBuilder[O, M] {
	return d.DefaultSystemPromptTextWihtParams(text, nil)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M] {
	return d.DefaultSystemPrompt(
		NewDefaultSystemPrompt().
			SetText(text).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M] {
	d.request.SetSystemPrompt(systemPrompt)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) Build() ChatClient[O, M] {
	return NewDefaultChatClient(d.request)
}
