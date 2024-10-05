package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
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
	SetAdvisors(advisors Advisors) ChatClientRequest[O, M]
	Call() CallResponse[O, M]
	Stream() StreamResponse[O, M]
	Mutate() ChatClientBuilder[O, M]
}

func NewDefaultChatClientRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata]() *DefaultChatClientRequest[O, M] {
	return &DefaultChatClientRequest[O, M]{
		systemParams:       make(map[string]any),
		userParams:         make(map[string]any),
		advisorParams:      make(map[string]any),
		aroundAdvisorChain: advisor.NewDefaultAroundChain[O, M](),
	}
}

var _ ChatClientRequest[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultChatClientRequest[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultChatClientRequest[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	chatModel          model.ChatModel[O, M]
	chatOptions        O
	systemText         string
	systemParams       map[string]any
	userText           string
	userParams         map[string]any
	messages           []message.ChatMessage
	advisors           []api.Advisor
	advisorParams      map[string]any
	aroundAdvisorChain *advisor.DefaultAroundChain[O, M]
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

func (d *DefaultChatClientRequest[O, M]) SetAdvisors(advisors Advisors) ChatClientRequest[O, M] {
	d.advisors = append(d.advisors, advisors.Advisors()...)
	for k, v := range advisors.Params() {
		d.advisorParams[k] = v
	}
	d.aroundAdvisorChain.PushAroundAdvisors(advisors.Advisors()...)
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
		DefaultChatOptions(d.chatOptions).
		DefaultSystemPromptTextWihtParams(d.systemText, d.systemParams).
		DefaultUserPromptTextWihtParams(d.userText, d.userParams).
		DefaultAdvisorsWihtParams(d.advisorParams, d.advisors...).(*DefaultChatClientBuilder[O, M])

	builder.request.messages = append(builder.request.messages, d.messages...)

	return builder
}

func (d *DefaultChatClientRequest[O, M]) toAdvisedRequest() *api.AdvisedRequest[O, M] {
	return api.NewAdvisedRequestBuilder[O, M]().
		WithChatModel(d.chatModel).
		WithUserText(d.userText).
		WithSystemText(d.systemText).
		WithChatOptions(d.chatOptions).
		WithMessages(d.messages...).
		WithUserParam(d.userParams).
		WithSystemParam(d.systemParams).
		WithAdvisors(d.advisors...).
		WithAdvisorParam(d.advisorParams).
		Build()
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
		chatModel:          old.chatModel,
		userText:           old.userText,
		systemText:         old.systemText,
		chatOptions:        old.chatOptions,
		messages:           old.messages,
		userParams:         old.userParams,
		systemParams:       old.systemParams,
		advisors:           old.advisors,
		advisorParams:      old.advisorParams,
		aroundAdvisorChain: old.aroundAdvisorChain.Clone(),
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
func (b *DefaultChatClientRequestBuilder[O, M]) WithAdvisors(advisors ...api.Advisor) *DefaultChatClientRequestBuilder[O, M] {
	b.request.advisors = advisors
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WithAdvisorParams(advisorParams map[string]any) *DefaultChatClientRequestBuilder[O, M] {
	b.request.advisorParams = advisorParams
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) WihtAroundAdvisorChain(chain *advisor.DefaultAroundChain[O, M]) *DefaultChatClientRequestBuilder[O, M] {
	b.request.aroundAdvisorChain = chain
	return b
}
func (b *DefaultChatClientRequestBuilder[O, M]) Build() (*DefaultChatClientRequest[O, M], error) {
	return b.request, nil
}
