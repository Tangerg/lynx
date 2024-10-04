package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClientBuilder interface {
	DefaultChatOptions(options prompt.ChatOptions) ChatClientBuilder
	DefaultPrueAdvisors(advisors ...api.Advisor) ChatClientBuilder
	DefaultAdvisorsWihtParams(params map[string]any, advisors ...api.Advisor) ChatClientBuilder
	DefaultAdvisors(advisors Advisors) ChatClientBuilder
	DefaultUserPromptText(text string) ChatClientBuilder
	DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder
	DefaultUserPrompt(user UserPrompt) ChatClientBuilder
	DefaultSystemPromptText(text string) ChatClientBuilder
	DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder
	DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder
	Build() ChatClient
}

func NewDefaultChatClientBuilder(chatModel model.ChatModel[prompt.ChatOptions, metadata.ChatGenerationMetadata]) *DefaultChatClientBuilder {
	chain := advisor.
		NewDefaultAroundChain().
		PushAroundAdvisor(advisor.NewDialAdvisor())

	request, _ := NewDefaultChatClientRequestBuilder().
		WithChatModel(chatModel).
		WihtAroundAdvisorChain(chain).
		Build()

	return &DefaultChatClientBuilder{
		request: request,
	}
}

var _ ChatClientBuilder = (*DefaultChatClientBuilder)(nil)

type DefaultChatClientBuilder struct {
	request *DefaultChatClientRequest
}

func (d *DefaultChatClientBuilder) DefaultPrueAdvisors(advisors ...api.Advisor) ChatClientBuilder {
	return d.DefaultAdvisorsWihtParams(nil, advisors...)
}

func (d *DefaultChatClientBuilder) DefaultAdvisorsWihtParams(params map[string]any, advisors ...api.Advisor) ChatClientBuilder {
	return d.DefaultAdvisors(
		NewDefaultAdvisors().
			SetAdvisors(advisors...).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder) DefaultAdvisors(advisors Advisors) ChatClientBuilder {
	d.request.SetAdvisors(advisors)
	return d
}

func (d *DefaultChatClientBuilder) DefaultChatOptions(options prompt.ChatOptions) ChatClientBuilder {
	d.request.SetChatOptions(options)
	return d
}

func (d *DefaultChatClientBuilder) DefaultUserPromptText(text string) ChatClientBuilder {
	return d.DefaultUserPromptTextWihtParams(text, nil)
}

func (d *DefaultChatClientBuilder) DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder {
	return d.DefaultUserPrompt(
		NewDefaultUserPrompt().
			SetText(text).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder) DefaultUserPrompt(user UserPrompt) ChatClientBuilder {
	d.request.SetUserPrompt(user)
	return d
}

func (d *DefaultChatClientBuilder) DefaultSystemPromptText(text string) ChatClientBuilder {
	return d.DefaultSystemPromptTextWihtParams(text, nil)
}

func (d *DefaultChatClientBuilder) DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder {
	return d.DefaultSystemPrompt(
		NewDefaultSystemPrompt().
			SetText(text).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder) DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder {
	d.request.SetSystemPrompt(systemPrompt)
	return d
}

func (d *DefaultChatClientBuilder) Build() ChatClient {
	return NewDefaultChatClient(d.request)
}
