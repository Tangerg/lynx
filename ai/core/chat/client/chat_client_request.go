package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClientRequestSpec interface {
	Advisors(advisors ...api.Advisor) ChatClientRequestSpec
	Messages(messages ...message.ChatMessage) ChatClientRequestSpec
	Options(options prompt.ChatOptions) ChatClientRequestSpec
	System(system string) ChatClientRequestSpec
	User(user string) ChatClientRequestSpec
	Call() CallResponseSpec
	Stream() StreamResponseSpec
}

var _ ChatClientRequestSpec = (*DefaultChatClientRequest)(nil)

type DefaultChatClientRequest struct {
	chatModel          model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]
	userText           string
	systemText         string
	chatOptions        prompt.ChatOptions
	messages           []message.ChatMessage
	userParams         map[string]any
	systemParams       map[string]any
	advisors           []api.Advisor
	advisorParams      map[string]any
	aroundAdvisorChain *advisor.DefaultAroundChain
}

func (d *DefaultChatClientRequest) Advisors(advisors ...api.Advisor) ChatClientRequestSpec {
	d.advisors = append(d.advisors, advisors...)
	d.aroundAdvisorChain.PushAroundAdvisors(advisors...)
	return d
}

func (d *DefaultChatClientRequest) Messages(messages ...message.ChatMessage) ChatClientRequestSpec {
	d.messages = append(d.messages, messages...)
	return d
}

func (d *DefaultChatClientRequest) Options(options prompt.ChatOptions) ChatClientRequestSpec {
	d.chatOptions = options
	return d
}

func (d *DefaultChatClientRequest) System(system string) ChatClientRequestSpec {
	d.systemText = system
	return d
}

func (d *DefaultChatClientRequest) User(user string) ChatClientRequestSpec {
	d.userText = user
	return d
}

func (d *DefaultChatClientRequest) Call() CallResponseSpec {
	return NewDefaultCallResponseSpec(d)
}

func (d *DefaultChatClientRequest) Stream() StreamResponseSpec {
	return NewDefaultStreamResponseSpec(d)
}

func (d *DefaultChatClientRequest) ToAdvisedRequest() *api.AdvisedRequest {
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
	chain := advisor.NewDefaultAroundChain()
	chain.PushAroundAdvisor(newRequestAdvisor())
	return &DefaultChatClientRequestBuilder{
		request: &DefaultChatClientRequest{
			aroundAdvisorChain: chain,
		},
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

func (b *DefaultChatClientRequestBuilder) Build() (*DefaultChatClientRequest, error) {
	return b.request, nil
}
