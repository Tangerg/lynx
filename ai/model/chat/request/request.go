package request

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Request[[]messages.Message, ChatOptions] = (*ChatRequestBase[ChatOptions])(nil)

// ChatRequest is a convenient type alias for ChatRequestBase with standard ChatOptions.
// This provides a simpler way to use the most common chat request configuration
// without needing to specify the generic type parameter explicitly.
type ChatRequest = ChatRequestBase[ChatOptions]

// ChatRequestBase represents a generic chat request used in AI model requests.
// A chat request consists of one or more messages and additional chat options.
type ChatRequestBase[O ChatOptions] struct {
	messages []messages.Message
	options  O
}

// Instructions returns the list of messages that make up the chat request instructions.
func (c *ChatRequestBase[O]) Instructions() []messages.Message {
	return c.messages
}

// Options returns the chat options for this request.
func (c *ChatRequestBase[O]) Options() O {
	return c.options
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
	return &ChatRequestBase[ChatOptions]{
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
