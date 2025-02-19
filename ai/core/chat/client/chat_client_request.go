package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	baseModel "github.com/Tangerg/lynx/ai/core/model"
	"github.com/Tangerg/lynx/ai/core/model/media"
)

// ChatClientRequest is a generic interface that defines the contract for building and
// executing chat client requests in a chat application. It supports both call-based
// and stream-based request modes and provides methods for configuring the request
// parameters and behavior.
//
// Type Parameters:
//   - O: Represents the chat options, typically defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, typically defined
//     by result.ChatResultMetadata.
//
// Methods:
//
// SetChatModel:
//
//	SetChatModel(model model.ChatModel[O, M]) ChatClientRequest[O, M]
//	Sets the chat model to be used for processing the request.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetChatOptions:
//
//	SetChatOptions(options O) ChatClientRequest[O, M]
//	Sets the chat options for the request.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetSystemPrompt:
//
//	SetSystemPrompt(system SystemPrompt) ChatClientRequest[O, M]
//	Sets the system prompt, which may include system-generated instructions or context.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetUserPrompt:
//
//	SetUserPrompt(user UserPrompt) ChatClientRequest[O, M]
//	Sets the user prompt, which includes the user's input or query.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetMessages:
//
//	SetMessages(messages ...message.ChatMessage) ChatClientRequest[O, M]
//	Sets the sequence of chat messages for the request.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetMiddlewares:
//
//	SetMiddlewares(middlewares Middlewares[O, M]) ChatClientRequest[O, M]
//	Sets the middleware functions to be executed during the request processing.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// SetStreamChunkHandler:
//
//	SetStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientRequest[O, M]
//	Sets the handler for processing streaming chunks during a stream-based request.
//	Returns:
//	  - ChatClientRequest[O, M]: The instance itself, enabling method chaining.
//
// Call:
//
//	Call() CallResponse[O, M]
//	Executes the request in a call-based mode, returning a `CallResponse`
//	containing the result.
//	Returns:
//	  - CallResponse[O, M]: A response containing the results of the call-based execution.
//
// Stream:
//
//	Stream() StreamResponse[O, M]
//	Executes the request in a stream-based mode, returning a `StreamResponse`
//	containing the result.
//	Returns:
//	  - StreamResponse[O, M]: A response containing the results of the stream-based execution.
//
// Mutate:
//
//	Mutate() ChatClientBuilder[O, M]
//	Returns a `ChatClientBuilder` instance, allowing further modifications
//	to the request configuration.
//	Returns:
//	  - ChatClientBuilder[O, M]: A builder for modifying the chat client request.
//
// Example Usage:
//
//	var chatRequest ChatClientRequest[ChatOptions, ChatMetadata]
//
//	response := chatRequest.
//	    SetChatModel(myChatModel).
//	    SetChatOptions(ChatOptions{Temperature: 0.7}).
//	    SetUserPrompt(UserPrompt{Text: "Hello, world!"}).
//	    Call()
//
//	content, err := response.Content(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Response Content:", content)
type ChatClientRequest[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	// SetChatModel Sets the chat model to be used for processing the request.
	SetChatModel(model model.ChatModel[O, M]) ChatClientRequest[O, M]

	// SetChatOptions Sets the chat options for the request.
	SetChatOptions(options O) ChatClientRequest[O, M]

	// SetSystemPrompt Sets the system prompt, which may include system-generated instructions or context.
	SetSystemPrompt(system SystemPrompt) ChatClientRequest[O, M]

	// SetUserPrompt Sets the user prompt, which includes the user's input or query.
	SetUserPrompt(user UserPrompt) ChatClientRequest[O, M]

	// SetMessages Sets the sequence of chat messages for the request.
	SetMessages(messages ...message.ChatMessage) ChatClientRequest[O, M]

	// SetMiddlewares Sets the middleware functions to be executed during the request processing.
	SetMiddlewares(middlewares Middlewares[O, M]) ChatClientRequest[O, M]

	// SetStreamChunkHandler Sets the handler for processing streaming chunks during a stream-based request.
	SetStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientRequest[O, M]

	// Call Executes the request in a call-based mode, returning a CallResponse.
	Call() CallResponse[O, M]

	// Stream Executes the request in a stream-based mode, returning a StreamResponse.
	Stream() StreamResponse[O, M]

	// Mutate Returns a ChatClientBuilder instance for modifying the request configuration.
	Mutate() ChatClientBuilder[O, M]
}

func NewDefaultChatClientRequest[O request.ChatRequestOptions, M result.ChatResultMetadata]() *DefaultChatClientRequest[O, M] {
	return &DefaultChatClientRequest[O, M]{
		systemParams:     make(map[string]any),
		userParams:       make(map[string]any),
		middlewareParams: make(map[string]any),
	}
}

var _ ChatClientRequest[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultChatClientRequest[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultChatClientRequest[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	chatModel          model.ChatModel[O, M]
	chatRequestOptions O
	systemText         string
	systemParams       map[string]any
	userText           string
	userParams         map[string]any
	userMedia          []*media.Media
	messages           []message.ChatMessage
	middlewares        []middleware.Middleware[O, M]
	middlewareParams   map[string]any
	streamChunkHandler baseModel.StreamChunkHandler[*response.ChatResponse[M]]
}

func (d *DefaultChatClientRequest[O, M]) SetChatModel(model model.ChatModel[O, M]) ChatClientRequest[O, M] {
	d.chatModel = model
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetChatOptions(options O) ChatClientRequest[O, M] {
	d.chatRequestOptions = options
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetSystemPrompt(systemPrompt SystemPrompt) ChatClientRequest[O, M] {
	d.systemText = systemPrompt.Text()
	for k, v := range systemPrompt.Params() {
		d.systemParams[k] = v
	}
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetUserPrompt(userPrompt UserPrompt) ChatClientRequest[O, M] {
	d.userText = userPrompt.Text()
	for k, v := range userPrompt.Params() {
		d.userParams[k] = v
	}
	d.userMedia = append(d.userMedia, userPrompt.Media()...)
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetMessages(messages ...message.ChatMessage) ChatClientRequest[O, M] {
	d.messages = append(d.messages, messages...)
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetMiddlewares(middlewares Middlewares[O, M]) ChatClientRequest[O, M] {
	d.middlewares = append(d.middlewares, middlewares.Middlewares()...)
	for k, v := range middlewares.Params() {
		d.middlewareParams[k] = v
	}
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetStreamChunkHandler(handler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) ChatClientRequest[O, M] {
	d.streamChunkHandler = handler
	return d
}

func (d *DefaultChatClientRequest[O, M]) Call() CallResponse[O, M] {
	return NewDefaultCallResponse[O, M](d)
}

func (d *DefaultChatClientRequest[O, M]) Stream() StreamResponse[O, M] {
	return NewDefaultStreamResponseSpec[O, M](d)
}

func (d *DefaultChatClientRequest[O, M]) Mutate() ChatClientBuilder[O, M] {
	builder := NewDefaultChatClientBuilder[O, M](d.chatModel).
		DefaultSystemPromptTextWithParams(d.systemText, d.systemParams).
		DefaultUserPromptTextWithParamsAndMedia(d.userText, d.userParams, d.userMedia...).
		DefaultMiddlewaresWithParams(d.middlewareParams, d.middlewares...).
		DefaultChatRequestOptions(d.chatRequestOptions).(*DefaultChatClientBuilder[O, M])

	builder.request.messages = append(builder.request.messages, d.messages...)

	return builder
}

func (d *DefaultChatClientRequest[O, M]) toMiddlewareRequest(mode middleware.ChatRequestMode) *middleware.Request[O, M] {
	return &middleware.Request[O, M]{
		ChatModel:          d.chatModel,
		ChatRequestOptions: d.chatRequestOptions,
		UserText:           d.userText,
		UserParams:         d.userParams,
		UserMedia:          d.userMedia,
		SystemText:         d.systemText,
		SystemParams:       d.systemParams,
		Messages:           d.messages,
		Mode:               mode,
		StreamChunkHandler: d.streamChunkHandler,
	}
}

func NewDefaultChatClientRequestBuilder[O request.ChatRequestOptions, M result.ChatResultMetadata]() *DefaultChatClientRequestBuilder[O, M] {
	return &DefaultChatClientRequestBuilder[O, M]{
		request: NewDefaultChatClientRequest[O, M](),
	}
}

type DefaultChatClientRequestBuilder[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func (b *DefaultChatClientRequestBuilder[O, M]) FromDefaultChatClientRequest(old *DefaultChatClientRequest[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.WithChatModel(old.chatModel).
		WithChatOptions(old.chatRequestOptions).
		WithSystemText(old.systemText).
		WithSystemParams(old.systemParams).
		WithUserText(old.userText).
		WithUserParam(old.userParams).
		WithUserMeida(old.userMedia...).
		WithMessages(old.messages...).
		WithMiddlewares(old.middlewares...).
		WithMiddlewareParams(old.middlewareParams).
		WithStreamChunkHandler(old.streamChunkHandler)
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithChatModel(chatModel model.ChatModel[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.request.chatModel = chatModel
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithChatOptions(options O) *DefaultChatClientRequestBuilder[O, M] {
	b.request.chatRequestOptions = options.Clone().(O)
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithSystemText(systemText string) *DefaultChatClientRequestBuilder[O, M] {
	b.request.systemText = systemText
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithSystemParams(systemParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	if b.request.systemParams == nil {
		b.request.systemParams = make(map[string]any)
	}
	for k, v := range systemParams {
		b.request.systemParams[k] = v
	}
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithUserText(userText string) *DefaultChatClientRequestBuilder[O, M] {
	b.request.userText = userText
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithUserParam(userParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	if b.request.userParams == nil {
		b.request.userParams = make(map[string]any)
	}
	for k, v := range userParams {
		b.request.userParams[k] = v
	}
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithUserMeida(media ...*media.Media) *DefaultChatClientRequestBuilder[O, M] {
	b.request.userMedia = append(b.request.userMedia, media...)
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithMessages(messages ...message.ChatMessage) *DefaultChatClientRequestBuilder[O, M] {
	b.request.messages = append(b.request.messages, messages...)
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithMiddlewares(middlewares ...middleware.Middleware[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.request.middlewares = append(b.request.middlewares, middlewares...)
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithMiddlewareParams(middlewareParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	if b.request.middlewareParams == nil {
		b.request.middlewareParams = make(map[string]any)
	}
	for k, v := range middlewareParams {
		b.request.middlewareParams[k] = v
	}
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithStreamChunkHandler(streamChunkHandler baseModel.StreamChunkHandler[*response.ChatResponse[M]]) *DefaultChatClientRequestBuilder[O, M] {
	b.request.streamChunkHandler = streamChunkHandler
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) Build() (*DefaultChatClientRequest[O, M], error) {
	return b.request, nil
}
