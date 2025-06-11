package request

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Request[[]messages.Message, ChatOptions] = (*ChatRequestWithOptions[ChatOptions])(nil)

// ChatRequest is a convenient type alias for ChatRequestWithOptions with standard ChatOptions.
// This provides a simpler way to use the most common chat request configuration
// without needing to specify the generic type parameter explicitly.
type ChatRequest = ChatRequestWithOptions[ChatOptions]

// ChatRequestWithOptions represents a generic chat request used in AI model requests.
// A chat request consists of one or more messages and additional chat options.
type ChatRequestWithOptions[O ChatOptions] struct {
	messages []messages.Message
	options  O
}

// Instructions returns the list of messages that make up the chat request instructions.
func (c *ChatRequestWithOptions[O]) Instructions() []messages.Message {
	return c.messages
}

// Options returns the chat options for this request.
func (c *ChatRequestWithOptions[O]) Options() O {
	return c.options
}

// AugmentLastUserMessage finds the last user message in the request and applies the given function to modify it.
// The function searches backwards through the message list to find the most recent user message,
// then applies the provided transformation function. If the function returns nil, the original message is preserved.
// This method is useful for dynamically adding context or modifying user input before sending to the AI model.
func (c *ChatRequestWithOptions[O]) AugmentLastUserMessage(fn func(*messages.UserMessage) *messages.UserMessage) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		message := c.messages[i]
		if message.Type().IsUser() {
			userMessage, ok := message.(*messages.UserMessage)
			if ok {
				userMessage = fn(userMessage)
				if userMessage != nil {
					c.messages[i] = userMessage
				}
			}
			break
		}
	}
}

// AugmentLastUserMessageText appends the provided text to the last user message in the request.
// The text is appended with a double newline separator ("\n\n") to ensure proper formatting.
// This is a convenience method that preserves the original message's media and metadata while
// only modifying the text content. If no user message is found, the operation has no effect.
func (c *ChatRequestWithOptions[O]) AugmentLastUserMessageText(text string) {
	c.AugmentLastUserMessage(func(userMessage *messages.UserMessage) *messages.UserMessage {
		return messages.NewUserMessage(userMessage.Text()+"\n\n"+text, userMessage.Media(), userMessage.Metadata())
	})
}

// AugmentLastSystemMessage finds the last system message in the request and applies the given function to modify it.
// The function searches backwards through the message list to find the most recent system message,
// then applies the provided transformation function. If the function returns nil, the original message is preserved.
// This method is commonly used to dynamically adjust system instructions or add contextual information
// to the AI model's behavior guidelines.
func (c *ChatRequestWithOptions[O]) AugmentLastSystemMessage(fn func(*messages.SystemMessage) *messages.SystemMessage) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		message := c.messages[i]
		if message.Type().IsSystem() {
			systemMessage, ok := message.(*messages.SystemMessage)
			if ok {
				systemMessage = fn(systemMessage)
				if systemMessage != nil {
					c.messages[i] = systemMessage
				}
			}
			break
		}
	}
}

// AugmentLastSystemMessageText appends the provided text to the last system message in the request.
// The text is appended with a double newline separator ("\n\n") to ensure proper formatting.
// This is a convenience method that preserves the original message's metadata while only modifying
// the text content. This is particularly useful for adding dynamic system instructions or
// contextual information without recreating the entire system message. If no system message is found,
// the operation has no effect.
func (c *ChatRequestWithOptions[O]) AugmentLastSystemMessageText(text string) {
	c.AugmentLastSystemMessage(func(userMessage *messages.SystemMessage) *messages.SystemMessage {
		return messages.NewSystemMessage(userMessage.Text()+"\n\n"+text, userMessage.Metadata())
	})
}

// NewChatRequest creates a new ChatRequest with the provided messages and optional chat options.
// The function accepts a slice of messages and variadic ChatOptions parameters.
// Returns an error if the messages slice is empty, as at least one message is required.
// If multiple options are provided, only the first non-nil option will be used.
func NewChatRequest(messages []messages.Message, options ...ChatOptions) (*ChatRequest, error) {
	if len(messages) == 0 {
		return nil, errors.New("at least one message is required")
	}
	var opt ChatOptions
	if len(options) > 0 && options[0] != nil {
		opt = options[0]
	}
	return &ChatRequestWithOptions[ChatOptions]{
		messages: slices.Clone(messages),
		options:  opt,
	}, nil
}

// ChatRequestBuilder provides a builder pattern for creating ChatRequest instances.
type ChatRequestBuilder struct {
	messages []messages.Message
	options  ChatOptions
}

// NewChatRequestBuilder creates a new ChatRequestBuilder
func NewChatRequestBuilder() *ChatRequestBuilder {
	return &ChatRequestBuilder{}
}

// WithMessages sets the messages for the chat request.
// If messages is nil, the current messages will remain unchanged.
func (c *ChatRequestBuilder) WithMessages(messages ...messages.Message) *ChatRequestBuilder {
	if len(messages) > 0 {
		c.messages = append(c.messages, messages...)
	}
	return c
}

// WithOptions sets the chat options for the request.
// If options is nil, the current options will remain unchanged.
func (c *ChatRequestBuilder) WithOptions(options ChatOptions) *ChatRequestBuilder {
	if options != nil {
		c.options = options.Clone()
	}
	return c
}

// Build creates a new ChatRequest instance with validation.
// Returns an error if the required fields are not set properly.
func (c *ChatRequestBuilder) Build() (*ChatRequest, error) {
	return NewChatRequest(c.messages, c.options)
}

// MustBuild creates a new ChatRequest instance and panics if there's an error.
// Use this method only when you're confident that the request is valid.
func (c *ChatRequestBuilder) MustBuild() *ChatRequest {
	request, err := c.Build()
	if err != nil {
		panic(err)
	}
	return request
}
