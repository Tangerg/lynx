package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type ChatClient[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] interface {
	Prompt() ChatClientRequest[O, M]
	PromptText(text string) ChatClientRequest[O, M]
	PromptPrompt(prompt *prompt.ChatPrompt[O]) ChatClientRequest[O, M]
	Mutate() ChatClientBuilder[O, M]
}

func NewDefaultChatClient[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](request *DefaultChatClientRequest[O, M]) *DefaultChatClient[O, M] {
	return &DefaultChatClient[O, M]{defaultRequest: request}
}

var _ ChatClient[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*DefaultChatClient[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type DefaultChatClient[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	defaultRequest *DefaultChatClientRequest[O, M]
}

func (d *DefaultChatClient[O, M]) Prompt() ChatClientRequest[O, M] {
	request, _ := NewDefaultChatClientRequestBuilder[O, M]().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()
	return request
}

func (d *DefaultChatClient[O, M]) PromptText(text string) ChatClientRequest[O, M] {
	p, _ := prompt.
		NewChatPromptBuilder[O]().
		WithContent(text).
		WithOptions(d.defaultRequest.chatOptions).
		Build()
	return d.PromptPrompt(p)
}

func (d *DefaultChatClient[O, M]) PromptPrompt(prompt *prompt.ChatPrompt[O]) ChatClientRequest[O, M] {
	spec, _ := NewDefaultChatClientRequestBuilder[O, M]().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()

	messages := prompt.Instructions()
	if len(messages) == 0 {
		return spec
	}

	lastMessage := messages[len(messages)-1]
	if lastMessage.Role().IsUser() {
		spec.SetUserPrompt(
			NewDefaultUserPrompt().
				SetText(lastMessage.Content()),
		)
		messages = messages[:len(messages)-1]
	}

	spec.SetMessages(messages...)

	return spec
}

func (d *DefaultChatClient[O, M]) Mutate() ChatClientBuilder[O, M] {
	return d.defaultRequest.Mutate()
}
