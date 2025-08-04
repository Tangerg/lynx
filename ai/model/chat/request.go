package chat

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/model"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ model.Request[[]messages.Message, Options] = (*request[Options])(nil)

// Request is a type alias for the most common LLM chat request configuration.
type Request = request[Options]

// NewRequest creates an LLM chat request with conversation messages and optional model parameters.
// Returns an error if no messages are provided, as LLMs require at least one input message.
// If multiple options are given, only the first non-nil option is used.
func NewRequest(msgs []messages.Message, options ...Options) (*Request, error) {
	validMsgs := messages.ExcludeNil(msgs)
	if len(validMsgs) == 0 {
		return nil, errors.New("at least one valid message is required")
	}

	return &request[Options]{
		messages: validMsgs,
		options:  pkgSlices.FirstOr(options, nil),
		fields:   make(map[string]any),
	}, nil
}

// request represents an LLM chat request containing conversation context and model parameters.
type request[O Options] struct {
	messages []messages.Message
	options  O
	fields   map[string]any
}

// Instructions returns the conversation messages that will be sent to the LLM.
func (c *request[O]) Instructions() []messages.Message {
	return c.messages
}

// Options returns the LLM model configuration parameters for this request.
func (c *request[O]) Options() O {
	return c.options
}

func (c *request[O]) Fields() map[string]any {
	return c.fields
}

func (c *request[O]) Get(key string) (any, bool) {
	val, ok := c.fields[key]
	return val, ok
}

func (c *request[O]) Set(key string, val any) {
	c.fields[key] = val
}

// AugmentLastUserMessageText appends additional context to the user's input before LLM processing.
// Text is appended with "\n\n" separator to maintain proper formatting for the LLM.
// Preserves the original message's media and metadata.
func (c *request[O]) AugmentLastUserMessageText(text string) {
	messages.AugmentTextLastMessageOfType(c.messages, messages.User, text)
}
