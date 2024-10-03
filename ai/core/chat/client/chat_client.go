package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClient interface {
	Prompt() ChatClientRequestSpec
	PromptText(text string) ChatClientRequestSpec
	PromptPrompt(prompt *prompt.ChatPrompt[prompt.ChatOptions]) ChatClientRequestSpec
}

var _ ChatClient = (*DefaultChatClient)(nil)

type DefaultChatClient struct {
	request *DefaultChatClientRequest
}

func (d *DefaultChatClient) Prompt() ChatClientRequestSpec {
	request, _ := NewDefaultChatClientRequestBuilder().
		FromDefaultChatClientRequest(d.request).
		Build()
	return request
}

func (d *DefaultChatClient) PromptText(text string) ChatClientRequestSpec {
	p, _ := prompt.
		NewChatPromptBuilder[prompt.ChatOptions]().
		WithContent(text).
		WithOptions(d.request.chatOptions).
		Build()
	return d.PromptPrompt(p)
}

func (d *DefaultChatClient) PromptPrompt(prompt *prompt.ChatPrompt[prompt.ChatOptions]) ChatClientRequestSpec {
	spec, _ := NewDefaultChatClientRequestBuilder().
		FromDefaultChatClientRequest(d.request).
		Build()
	if prompt.Options() != nil {
		spec.chatOptions = prompt.Options()
	}

	messages := prompt.Instructions()
	if len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		if lastMessage.Role() == message.User {
			spec.User(lastMessage.Content())
		}
		messages = messages[:len(messages)-1]
	}
	spec.Messages(messages...)

	return spec
}
