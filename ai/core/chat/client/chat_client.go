package client

import (
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

// ChatClient is a generic interface that defines the contract for interacting with a chat client
// in a chat application. It supports creating and configuring chat requests, allowing users
// to initiate chat interactions and modify the client's configuration.
//
// Type Parameters:
//   - O: Represents the chat options, typically defined by request.ChatRequestOptions.
//   - M: Represents the metadata associated with chat generation, typically defined
//     by result.ChatResultMetadata.
//
// Methods:
//
// Prompt:
//
//	Prompt() ChatClientRequest[O, M]
//	Initiates a new chat client request with default settings and returns a `ChatClientRequest`
//	instance. This method is used as a starting point for building chat requests.
//
// PromptText:
//
//	PromptText(text string) ChatClientRequest[O, M]
//	Initiates a new chat client request with the specified text as the user prompt.
//	Returns:
//	  - ChatClientRequest[O, M]: A new `ChatClientRequest` instance, allowing further
//	    configuration of the request.
//
// PromptRequest:
//
//	PromptRequest(prompt *request.ChatRequest[O]) ChatClientRequest[O, M]
//	Initiates a new chat client request using the specified `ChatRequest` instance.
//	This method supports complex configurations by allowing a fully defined `ChatRequest`
//	object to be used as input.
//	Returns:
//	  - ChatClientRequest[O, M]: A new `ChatClientRequest` instance, allowing further
//	    configuration of the request.
//
// Mutate:
//
//	Mutate() ChatClientBuilder[O, M]
//	Returns a `ChatClientBuilder` instance, enabling further modifications to the
//	chat client's configuration. This method is useful for altering the chat client's
//	settings or behavior before initiating requests.
//
// Example Usage:
//
//	var client ChatClient[ChatOptions, ChatMetadata]
//
//	// Create a simple chat request
//	request := client.PromptText("Hello, AI!")
//	response := request.Call()
//
//	// Modify the client configuration
//	client = client.Mutate().
//	    DefaultChatRequestOptions(ChatOptions{Temperature: 0.5}).
//	    Build()
//
//	// Create a complex chat request
//	chatRequest := &request.ChatRequest[ChatOptions]{
//	    ChatRequestOptions: ChatOptions{Temperature: 0.7},
//	    UserText: "What's the weather today?",
//	}
//	response = client.PromptRequest(chatRequest).Call()
type ChatClient[O request.ChatRequestOptions, M result.ChatResultMetadata] interface {
	// Prompt Initiates a new chat client request with default settings.
	Prompt() ChatClientRequest[O, M]

	// PromptText Initiates a new chat client request with the specified text as the user prompt.
	PromptText(text string) ChatClientRequest[O, M]

	// PromptRequest Initiates a new chat client request using the specified ChatRequest instance.
	PromptRequest(prompt *request.ChatRequest[O]) ChatClientRequest[O, M]

	// Mutate Returns a ChatClientBuilder instance, allowing modifications to the chat client configuration.
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
	return d.PromptRequest(p)
}

func (d *DefaultChatClient[O, M]) PromptRequest(prompt *request.ChatRequest[O]) ChatClientRequest[O, M] {
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
