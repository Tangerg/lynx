package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClientRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	SetChatModel(model model.ChatModel[O, M]) ChatClientRequest[O, M]
	SetChatOptions(options O) ChatClientRequest[O, M]
	SetSystemPrompt(system SystemPrompt) ChatClientRequest[O, M]
	SetUserPrompt(user UserPrompt) ChatClientRequest[O, M]
	SetMessages(messages ...message.ChatMessage) ChatClientRequest[O, M]
	SetMiddlewares(middlewares Middlewares[O, M]) ChatClientRequest[O, M]
	Call() CallResponse[O, M]
	Stream() StreamResponse[O, M]
	Mutate() ChatClientBuilder[O, M]
}

func NewDefaultChatClientRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DefaultChatClientRequest[O, M] {
	return &DefaultChatClientRequest[O, M]{
		systemParams:     make(map[string]any),
		userParams:       make(map[string]any),
		middlewareParams: make(map[string]any),
	}
}

var _ ChatClientRequest[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultChatClientRequest[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultChatClientRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	chatModel        model.ChatModel[O, M]
	chatOptions      O
	systemText       string
	systemParams     map[string]any
	userText         string
	userParams       map[string]any
	messages         []message.ChatMessage
	middlewares      []middleware.Middleware[O, M]
	middlewareParams map[string]any
}

func (d *DefaultChatClientRequest[O, M]) SetChatModel(model model.ChatModel[O, M]) ChatClientRequest[O, M] {
	d.chatModel = model
	return d
}

func (d *DefaultChatClientRequest[O, M]) SetChatOptions(options O) ChatClientRequest[O, M] {
	d.chatOptions = options
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

func (d *DefaultChatClientRequest[O, M]) Call() CallResponse[O, M] {
	return NewDefaultCallResponse[O, M](d)
}

func (d *DefaultChatClientRequest[O, M]) Stream() StreamResponse[O, M] {
	return NewDefaultStreamResponseSpec[O, M](d)
}

func (d *DefaultChatClientRequest[O, M]) Mutate() ChatClientBuilder[O, M] {
	builder := NewDefaultChatClientBuilder[O, M](d.chatModel).
		DefaultSystemPromptTextWihtParams(d.systemText, d.systemParams).
		DefaultUserPromptTextWihtParams(d.userText, d.userParams).
		DefaultMiddlewaresWithParams(d.middlewareParams, d.middlewares...).
		DefaultChatOptions(d.chatOptions).(*DefaultChatClientBuilder[O, M])

	builder.request.messages = append(builder.request.messages, d.messages...)

	return builder
}

func (d *DefaultChatClientRequest[O, M]) toMiddlewareRequest() *middleware.Request[O, M] {
	return &middleware.Request[O, M]{
		ChatModel:    d.chatModel,
		ChatOptions:  d.chatOptions,
		UserText:     d.userText,
		UserParams:   d.userParams,
		SystemText:   d.systemText,
		SystemParams: d.systemParams,
		Messages:     d.messages,
		Mode:         model.CallRequest,
	}
}

func NewDefaultChatClientRequestBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DefaultChatClientRequestBuilder[O, M] {
	return &DefaultChatClientRequestBuilder[O, M]{
		request: NewDefaultChatClientRequest[O, M](),
	}
}

type DefaultChatClientRequestBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func (b *DefaultChatClientRequestBuilder[O, M]) FromDefaultChatClientRequest(old *DefaultChatClientRequest[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.request = &DefaultChatClientRequest[O, M]{
		chatModel:        old.chatModel,
		userText:         old.userText,
		systemText:       old.systemText,
		chatOptions:      old.chatOptions,
		messages:         old.messages,
		userParams:       old.userParams,
		systemParams:     old.systemParams,
		middlewares:      old.middlewares,
		middlewareParams: old.middlewareParams,
	}
	return b
}

func (b *DefaultChatClientRequestBuilder[O, M]) WithChatModel(chatModel model.ChatModel[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.request.chatModel = chatModel
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithChatOptions(options O) *DefaultChatClientRequestBuilder[O, M] {
	b.request.chatOptions = options
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithUserText(userText string) *DefaultChatClientRequestBuilder[O, M] {
	b.request.userText = userText
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithUserParam(userParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	b.request.userParams = userParams
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithSystemText(systemText string) *DefaultChatClientRequestBuilder[O, M] {
	b.request.systemText = systemText
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithSystemParams(systemParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	b.request.systemParams = systemParams
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithMessages(messages ...message.ChatMessage) *DefaultChatClientRequestBuilder[O, M] {
	b.request.messages = messages
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) Build() (*DefaultChatClientRequest[O, M], error) {
	return b.request, nil
}
