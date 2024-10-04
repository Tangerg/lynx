package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClient interface {
	Prompt() ChatClientRequest
	PromptText(text string) ChatClientRequest
	PromptPrompt(prompt *prompt.ChatPrompt[prompt.ChatOptions]) ChatClientRequest
	Mutate() ChatClientBuilder
}

func NewDefaultChatClient(request *DefaultChatClientRequest) *DefaultChatClient {
	return &DefaultChatClient{defaultRequest: request}
}

var _ ChatClient = (*DefaultChatClient)(nil)

type DefaultChatClient struct {
	defaultRequest *DefaultChatClientRequest
}

func (d *DefaultChatClient) Prompt() ChatClientRequest {
	request, _ := NewDefaultChatClientRequestBuilder().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()
	return request
}

func (d *DefaultChatClient) PromptText(text string) ChatClientRequest {
	p, _ := prompt.
		NewChatPromptBuilder[prompt.ChatOptions]().
		WithContent(text).
		WithOptions(d.defaultRequest.chatOptions).
		Build()
	return d.PromptPrompt(p)
}

func (d *DefaultChatClient) PromptPrompt(prompt *prompt.ChatPrompt[prompt.ChatOptions]) ChatClientRequest {
	spec, _ := NewDefaultChatClientRequestBuilder().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()

	if prompt.Options() != nil {
		spec.SetChatOptions(prompt.Options())
	}

	messages := prompt.Instructions()
	if len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Role() == message.User {
			spec.SetUserPrompt(
				NewDefaultUserPrompt().
					SetText(lastMessage.Content()),
			)
		}
		messages = messages[:len(messages)-1]
	}

	spec.SetMessages(messages...)

	return spec
}

func (d *DefaultChatClient) Mutate() ChatClientBuilder {
	return d.defaultRequest.Mutate()
}
