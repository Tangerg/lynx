package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	baseModel "github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

// ChatClientBuilder is a generic interface that defines the contract for building and
// configuring a chat client in a chat application. It supports a flexible configuration
// process, allowing the user to set default options, prompts, middlewares, and other
// settings before creating a fully functional chat client.
//
// Type Parameters:
//   - O: Represents the chat options, typically defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, typically defined
//     by result.ChatResultMetadata.
//
// Methods:
//
// DefaultChatRequestOptions:
//
//	DefaultChatRequestOptions(options O) ChatClientBuilder[O, M]
//	Sets the default chat request options for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultMiddlewares:
//
//	DefaultMiddlewares(middlewares Middlewares[O, M]) ChatClientBuilder[O, M]
//	Sets the default middleware functions for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultMiddlewaresWithParams:
//
//	DefaultMiddlewaresWithParams(params map[string]any, middlewares ...middleware.Middleware[O, M]) ChatClientBuilder[O, M]
//	Sets the default middleware functions with additional parameters for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultUserPromptText:
//
//	DefaultUserPromptText(text string) ChatClientBuilder[O, M]
//	Sets the default user prompt text for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultUserPromptTextWithParams:
//
//	DefaultUserPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M]
//	Sets the default user prompt text with additional parameters for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultUserPromptTextWithParamsAndMedia:
//
//	DefaultUserPromptTextWithParamsAndMedia(text string, params map[string]any, media ...*media.Media) ChatClientBuilder[O, M]
//	Sets the default user prompt text with additional parameters and media inputs
//	for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultUserPrompt:
//
//	DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M]
//	Sets the default user prompt using a UserPrompt object.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultSystemPromptText:
//
//	DefaultSystemPromptText(text string) ChatClientBuilder[O, M]
//	Sets the default system prompt text for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultSystemPromptTextWithParams:
//
//	DefaultSystemPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M]
//	Sets the default system prompt text with additional parameters for the chat client.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultSystemPrompt:
//
//	DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M]
//	Sets the default system prompt using a SystemPrompt object.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// DefaultStreamChunkHandler:
//
//	DefaultStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientBuilder[O, M]
//	Sets the default stream chunk handler for the chat client, used in streaming mode.
//	Returns:
//	  - ChatClientBuilder[O, M]: The builder instance to enable method chaining.
//
// Build:
//
//	Build() ChatClient[O, M]
//	Finalizes the configuration process and constructs a ChatClient instance configured
//	with the specified defaults.
//	Returns:
//	  - ChatClient[O, M]: A fully configured chat client ready for use.
//
// Example Usage:
//
//	builder := NewChatClientBuilder[ChatOptions, ChatMetadata]()
//	client := builder.
//	    DefaultChatRequestOptions(ChatOptions{Temperature: 0.7}).
//	    DefaultUserPromptText("Hello!").
//	    DefaultSystemPromptText("Welcome to the AI system.").
//	    Build()
//
//	// Use the configured client to execute chat requests.
type ChatClientBuilder[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	// DefaultChatRequestOptions Sets the default chat request options for the chat client.
	DefaultChatRequestOptions(options O) ChatClientBuilder[O, M]

	// DefaultMiddlewares Sets the default middleware functions for the chat client.
	DefaultMiddlewares(middlewares Middlewares[O, M]) ChatClientBuilder[O, M]

	// DefaultMiddlewaresWithParams Sets the default middleware functions with additional parameters for the chat client.
	DefaultMiddlewaresWithParams(params map[string]any, middlewares ...middleware.Middleware[O, M]) ChatClientBuilder[O, M]

	// DefaultUserPromptText Sets the default user prompt text for the chat client.
	DefaultUserPromptText(text string) ChatClientBuilder[O, M]

	// DefaultUserPromptTextWithParams Sets the default user prompt text with additional parameters for the chat client.
	DefaultUserPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M]

	// DefaultUserPromptTextWithParamsAndMedia Sets the default user prompt text with additional parameters and media inputs for the chat client.
	DefaultUserPromptTextWithParamsAndMedia(text string, params map[string]any, media ...*media.Media) ChatClientBuilder[O, M]

	// DefaultUserPrompt Sets the default user prompt using a UserPrompt object.
	DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M]

	// DefaultSystemPromptText Sets the default system prompt text for the chat client.
	DefaultSystemPromptText(text string) ChatClientBuilder[O, M]

	// DefaultSystemPromptTextWithParams Sets the default system prompt text with additional parameters for the chat client.
	DefaultSystemPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M]

	// DefaultSystemPrompt Sets the default system prompt using a SystemPrompt object.
	DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M]

	// DefaultStreamChunkHandler Sets the default stream chunk handler for the chat client.
	DefaultStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientBuilder[O, M]

	// Build Finalizes the configuration and constructs a ChatClient instance.
	Build() ChatClient[O, M]
}

func NewDefaultChatClientBuilder[O request.ChatRequestOptions, M result.ChatResultMetadata](chatModel model.ChatModel[O, M]) *DefaultChatClientBuilder[O, M] {

	req, _ := NewDefaultChatClientRequestBuilder[O, M]().
		WithChatModel(chatModel).
		Build()

	return &DefaultChatClientBuilder[O, M]{
		request: req,
	}
}

var _ ChatClientBuilder[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultChatClientBuilder[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultChatClientBuilder[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
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

func (d *DefaultChatClientBuilder[O, M]) DefaultChatRequestOptions(options O) ChatClientBuilder[O, M] {
	d.request.SetChatOptions(options)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptText(text string) ChatClientBuilder[O, M] {
	return d.DefaultUserPromptTextWithParams(text, nil)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M] {
	return d.DefaultUserPromptTextWithParamsAndMedia(text, params)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultUserPromptTextWithParamsAndMedia(text string, params map[string]any, media ...*media.Media) ChatClientBuilder[O, M] {
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
	return d.DefaultSystemPromptTextWithParams(text, nil)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultSystemPromptTextWithParams(text string, params map[string]any) ChatClientBuilder[O, M] {
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

func (d *DefaultChatClientBuilder[O, M]) DefaultStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientBuilder[O, M] {
	d.request.SetStreamChunkHandler(handler)
	return d
}

func (d *DefaultChatClientBuilder[O, M]) Build() ChatClient[O, M] {
	return NewDefaultChatClient(d.request)
}
