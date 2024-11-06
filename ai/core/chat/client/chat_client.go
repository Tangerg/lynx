package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// ChatClient is a generic interface that defines the contract for interacting with a chat client
// in a chat application. It is parameterized by chat options (O) and chat generation metadata (M).
//
// Type Parameters:
//   - O: Represents the chat options, defined by the prompt.ChatOptions type.
//   - M: Represents the metadata associated with chat generation, defined by the metadata.ChatGenerationMetadata type.
//
// Methods:
//
// Prompt() ChatClientRequest[O, M]
//   - Initiates a new chat client request and returns a ChatClientRequest instance.
//   - This method is used to start building a chat request with default settings.
//
// PromptText(text string) ChatClientRequest[O, M]
//   - Initiates a new chat client request with the specified text as the user prompt.
//   - Returns a ChatClientRequest instance to allow further configuration of the request.
//
// PromptPrompt(prompt *prompt.ChatPrompt[O]) ChatClientRequest[O, M]
//   - Initiates a new chat client request using the specified ChatPrompt instance.
//   - Returns a ChatClientRequest instance to allow further configuration of the request.
//   - This method allows for more complex prompt configurations using a ChatPrompt object.
//
// Mutate() ChatClientBuilder[O, M]
//   - Returns a ChatClientBuilder instance, allowing further modifications to the chat client configuration.
//   - This method is useful for altering the chat client's settings or behavior before initiating requests.
type ChatClient[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	Prompt() ChatClientRequest[O, M]
	PromptText(text string) ChatClientRequest[O, M]
	PromptPrompt(prompt *request.ChatRequest[O]) ChatClientRequest[O, M]
	Mutate() ChatClientBuilder[O, M]
}

func NewDefaultChatClient[O request.ChatRequestOptions, M result.ChatResultMetadata](request *DefaultChatClientRequest[O, M]) *DefaultChatClient[O, M] {
	return &DefaultChatClient[O, M]{defaultRequest: request}
}

var _ ChatClient[request.ChatRequestOptions, result.ChatResultMetadata] = (*DefaultChatClient[request.ChatRequestOptions, result.ChatResultMetadata])(nil)

type DefaultChatClient[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	defaultRequest *DefaultChatClientRequest[O, M]
}

func (d *DefaultChatClient[O, M]) Prompt() ChatClientRequest[O, M] {
	req, _ := NewDefaultChatClientRequestBuilder[O, M]().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()
	return req
}

func (d *DefaultChatClient[O, M]) PromptText(text string) ChatClientRequest[O, M] {
	p, _ := request.
		NewChatRequestBuilder[O]().
		WithContent(text).
		WithOptions(d.defaultRequest.chatRequestOptions).
		Build()
	return d.PromptPrompt(p)
}

func (d *DefaultChatClient[O, M]) PromptPrompt(prompt *request.ChatRequest[O]) ChatClientRequest[O, M] {
	spec, _ := NewDefaultChatClientRequestBuilder[O, M]().
		FromDefaultChatClientRequest(d.defaultRequest).
		Build()

	messages := prompt.Instructions()
	if len(messages) == 0 {
		return spec
	}

	lastMessage := messages[len(messages)-1]
	if lastMessage.Type().IsUser() {
		usermessage := lastMessage.(*message.UserMessage)
		spec.SetUserPrompt(
			NewDefaultUserPrompt().
				SetText(usermessage.Content()).
				SetParams(usermessage.Metadata()).
				SetMedia(usermessage.Media()...),
		)
		messages = messages[:len(messages)-1]
	}

	spec.SetMessages(messages...)

	return spec
}

func (d *DefaultChatClient[O, M]) Mutate() ChatClientBuilder[O, M] {
	return d.defaultRequest.Mutate()
}
