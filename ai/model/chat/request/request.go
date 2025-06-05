package request

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
)

var _ model.Request[[]messages.Message, ChatOptions] = (*ChatRequest[ChatOptions])(nil)

// ChatRequest represents a prompt used in AI model requests. A prompt consists of
// one or more messages and additional chat options.
type ChatRequest[O ChatOptions] struct {
	messages []messages.Message
	options  O
}

// Instructions returns the list of messages that make up the chat request instructions.
func (c *ChatRequest[O]) Instructions() []messages.Message {
	return c.messages
}

// Options returns the chat options for this request.
func (c *ChatRequest[O]) Options() O {
	return c.options
}

// NewChatRequest creates a new ChatRequest with the given messages and options.
// Returns an error if messages or options are nil.
func NewChatRequest(messages []messages.Message, options ChatOptions) (*ChatRequest[ChatOptions], error) {
	if messages == nil {
		return nil, errors.New("messages is required")
	}
	if len(messages) == 0 {
		return nil, errors.New("at least one message is required")
	}
	if options == nil {
		return nil, errors.New("options is required")
	}
	return &ChatRequest[ChatOptions]{
		messages: messages,
		options:  options,
	}, nil
}

// ChatRequestBuilder provides a builder pattern for creating ChatRequest instances.
type ChatRequestBuilder struct {
	messages []messages.Message
	options  ChatOptions
}

// WithMessages sets the messages for the chat request.
// If messages is nil, the current messages will remain unchanged.
func (c *ChatRequestBuilder) WithMessages(messages []messages.Message) *ChatRequestBuilder {
	if messages != nil {
		c.messages = messages
	}
	return c
}

// WithOptions sets the chat options for the request.
// If options is nil, the current options will remain unchanged.
func (c *ChatRequestBuilder) WithOptions(options ChatOptions) *ChatRequestBuilder {
	if options != nil {
		c.options = options
	}
	return c
}

// Build creates a new ChatRequest instance with validation.
// Returns an error if the required fields are not set properly.
func (c *ChatRequestBuilder) Build() (*ChatRequest[ChatOptions], error) {
	return NewChatRequest(c.messages, c.options)
}

// MustBuild creates a new ChatRequest instance and panics if there's an error.
// Use this method only when you're confident that the request is valid.
func (c *ChatRequestBuilder) MustBuild() *ChatRequest[ChatOptions] {
	request, err := c.Build()
	if err != nil {
		panic(err)
	}
	return request
}
