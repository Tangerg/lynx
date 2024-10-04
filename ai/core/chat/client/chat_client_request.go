package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClientRequest interface {
	SetChatModel(model model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]) ChatClientRequest
	SetChatOptions(options prompt.ChatOptions) ChatClientRequest
	SetSystemPrompt(system SystemPrompt) ChatClientRequest
	SetUserPrompt(user UserPrompt) ChatClientRequest
	SetMessages(messages ...message.ChatMessage) ChatClientRequest
	SetAdvisors(advisors Advisors) ChatClientRequest
	Call() CallResponse
	Stream() StreamResponse
	Mutate() ChatClientBuilder
}

var _ ChatClientRequest = (*DefaultChatClientRequest)(nil)

type DefaultChatClientRequest struct {
	chatModel          model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]
	chatOptions        prompt.ChatOptions
	systemText         string
	systemParams       map[string]any
	userText           string
	userParams         map[string]any
	messages           []message.ChatMessage
	advisors           []api.Advisor
	advisorParams      map[string]any
	aroundAdvisorChain *advisor.DefaultAroundChain
}

func (d *DefaultChatClientRequest) SetChatModel(model model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]) ChatClientRequest {
	d.chatModel = model
	return d
}

func (d *DefaultChatClientRequest) SetChatOptions(options prompt.ChatOptions) ChatClientRequest {
	d.chatOptions = options
	return d
}

func (d *DefaultChatClientRequest) SetSystemPrompt(systemPrompt SystemPrompt) ChatClientRequest {
	d.systemText = systemPrompt.Text()
	for k, v := range systemPrompt.Params() {
		d.systemParams[k] = v
	}
	return d
}

func (d *DefaultChatClientRequest) SetUserPrompt(userPrompt UserPrompt) ChatClientRequest {
	d.userText = userPrompt.Text()
	for k, v := range userPrompt.Params() {
		d.userParams[k] = v
	}
	return d
}

func (d *DefaultChatClientRequest) SetMessages(messages ...message.ChatMessage) ChatClientRequest {
	d.messages = append(d.messages, messages...)
	return d
}

func (d *DefaultChatClientRequest) SetAdvisors(advisors Advisors) ChatClientRequest {
	d.advisors = append(d.advisors, advisors.Advisors()...)
	for k, v := range advisors.Params() {
		d.advisorParams[k] = v
	}
	d.aroundAdvisorChain.PushAroundAdvisors(advisors.Advisors()...)
	return d
}

func (d *DefaultChatClientRequest) Call() CallResponse {
	return NewDefaultCallResponseSpec(d)
}

func (d *DefaultChatClientRequest) Stream() StreamResponse {
	return NewDefaultStreamResponseSpec(d)
}

func (d *DefaultChatClientRequest) Mutate() ChatClientBuilder {
	builder := NewDefaultChatClientBuilder(d.chatModel).
		DefaultChatOptions(d.chatOptions).
		DefaultSystemPromptTextWihtParams(d.systemText, d.systemParams).
		DefaultUserPromptTextWihtParams(d.userText, d.userParams).
		DefaultAdvisorsWihtParams(d.advisorParams, d.advisors...).(*DefaultChatClientBuilder)

	builder.request.messages = append(builder.request.messages, d.messages...)

	return builder
}

func (d *DefaultChatClientRequest) toAdvisedRequest() *api.AdvisedRequest {
	return api.NewAdvisedRequestBuilder().
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

type DefaultChatClientRequestBuilder struct {
	request *DefaultChatClientRequest
}

func NewDefaultChatClientRequestBuilder() *DefaultChatClientRequestBuilder {
	return &DefaultChatClientRequestBuilder{
		request: &DefaultChatClientRequest{},
	}
}

func (b *DefaultChatClientRequestBuilder) FromDefaultChatClientRequest(old *DefaultChatClientRequest) *DefaultChatClientRequestBuilder {
	b.request = &DefaultChatClientRequest{
		chatModel:          old.chatModel,
		userText:           old.userText,
		systemText:         old.systemText,
		chatOptions:        old.chatOptions,
		messages:           old.messages,
		userParams:         old.userParams,
		systemParams:       old.systemParams,
		advisors:           old.advisors,
		advisorParams:      old.advisorParams,
		aroundAdvisorChain: old.aroundAdvisorChain,
	}
	return b
}

func (b *DefaultChatClientRequestBuilder) WithChatModel(chatModel model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]) *DefaultChatClientRequestBuilder {
	b.request.chatModel = chatModel
	return b
}
func (b *DefaultChatClientRequestBuilder) WithChatOptions(options prompt.ChatOptions) *DefaultChatClientRequestBuilder {
	b.request.chatOptions = options
	return b
}
func (b *DefaultChatClientRequestBuilder) WithUserText(userText string) *DefaultChatClientRequestBuilder {
	b.request.userText = userText
	return b
}
func (b *DefaultChatClientRequestBuilder) WithUserParam(userParams map[string]any) *DefaultChatClientRequestBuilder {
	b.request.userParams = userParams
	return b
}
func (b *DefaultChatClientRequestBuilder) WithSystemText(systemText string) *DefaultChatClientRequestBuilder {
	b.request.systemText = systemText
	return b
}
func (b *DefaultChatClientRequestBuilder) WithSystemParams(systemParams map[string]any) *DefaultChatClientRequestBuilder {
	b.request.systemParams = systemParams
	return b
}
func (b *DefaultChatClientRequestBuilder) WithMessages(messages ...message.ChatMessage) *DefaultChatClientRequestBuilder {
	b.request.messages = messages
	return b
}
func (b *DefaultChatClientRequestBuilder) WithAdvisors(advisors ...api.Advisor) *DefaultChatClientRequestBuilder {
	b.request.advisors = advisors
	return b
}
func (b *DefaultChatClientRequestBuilder) WithAdvisorParams(advisorParams map[string]any) *DefaultChatClientRequestBuilder {
	b.request.advisorParams = advisorParams
	return b
}
func (b *DefaultChatClientRequestBuilder) WihtAroundAdvisorChain(chain *advisor.DefaultAroundChain) *DefaultChatClientRequestBuilder {
	b.request.aroundAdvisorChain = chain
	return b
}
func (b *DefaultChatClientRequestBuilder) Build() (*DefaultChatClientRequest, error) {
	return b.request, nil
}
