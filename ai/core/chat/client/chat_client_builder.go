package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor"
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/model"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	DefaultChatOptions(options O) ChatClientBuilder[O, M]
	DefaultPrueAdvisors(advisors ...api.Advisor) ChatClientBuilder[O, M]
	DefaultAdvisorsWihtParams(params map[string]any, advisors ...api.Advisor) ChatClientBuilder[O, M]
	DefaultAdvisors(advisors Advisors) ChatClientBuilder[O, M]
	DefaultUserPromptText(text string) ChatClientBuilder[O, M]
	DefaultUserPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
	DefaultUserPrompt(user UserPrompt) ChatClientBuilder[O, M]
	DefaultSystemPromptText(text string) ChatClientBuilder[O, M]
	DefaultSystemPromptTextWihtParams(text string, params map[string]any) ChatClientBuilder[O, M]
	DefaultSystemPrompt(systemPrompt SystemPrompt) ChatClientBuilder[O, M]
	Build() ChatClient[O, M]
}

func NewDefaultChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](chatModel model.ChatModel[O, M]) *DefaultChatClientBuilder[O, M] {
	chain := advisor.
		NewDefaultAroundChain[O, M]().
		PushAroundAdvisor(advisor.NewDialAdvisor[O, M]())

	request, _ := NewDefaultChatClientRequestBuilder[O, M]().
		WithChatModel(chatModel).
		WihtAroundAdvisorChain(chain).
		Build()

	return &DefaultChatClientBuilder[O, M]{
		request: request,
	}
}

var _ ChatClientBuilder[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultChatClientBuilder[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultChatClientBuilder[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	request *DefaultChatClientRequest[O, M]
}

func (d *DefaultChatClientBuilder[O, M]) DefaultPrueAdvisors(advisors ...api.Advisor) ChatClientBuilder[O, M] {
	return d.DefaultAdvisorsWihtParams(nil, advisors...)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultAdvisorsWihtParams(params map[string]any, advisors ...api.Advisor) ChatClientBuilder[O, M] {
	return d.DefaultAdvisors(
		NewDefaultAdvisors().
			SetAdvisors(advisors...).
			SetParams(params),
	)
}

func (d *DefaultChatClientBuilder[O, M]) DefaultAdvisors(advisors Advisors) ChatClientBuilder[O, M] {
	d.request.SetAdvisors(advisors)
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
	return d.DefaultUserPrompt(
		NewDefaultUserPrompt().
			SetText(text).
			SetParams(params),
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
