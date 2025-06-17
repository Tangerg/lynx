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
	if len(msgs) == 0 {
		return nil, errors.New("at least one message is required")
	}

	return &request[Options]{
		messages: messages.FilterNonNil(msgs),
		options:  pkgSlices.FirstOr(options, nil),
	}, nil
}

// request represents an LLM chat request containing conversation context and model parameters.
type request[O Options] struct {
	messages []messages.Message
	options  O
}

// Instructions returns the conversation messages that will be sent to the LLM.
func (c *request[O]) Instructions() []messages.Message {
	return c.messages
}

// Options returns the LLM model configuration parameters for this request.
func (c *request[O]) Options() O {
	return c.options
}

// findLastMessageByType searches backwards for the most recent message of the specified type.
// Returns the index and message if found, or (-1, nil) if not found.
func (c *request[O]) findLastMessageByType(typ messages.Type) (int, messages.Message) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		message := c.messages[i]
		if message.Type() == typ {
			return i, message
		}
	}
	return -1, nil
}

// AugmentLastUserMessage modifies the most recent user message before sending to the LLM.
// This is useful for adding context or instructions that affect how the LLM processes the request.
// If the function returns nil, the original message is preserved.
func (c *request[O]) AugmentLastUserMessage(fn func(*messages.UserMessage) *messages.UserMessage) {
	index, message := c.findLastMessageByType(messages.User)
	if index == -1 {
		return
	}

	userMessage, ok := message.(*messages.UserMessage)
	if ok {
		userMessage = fn(userMessage)
		if userMessage != nil {
			c.messages[index] = userMessage
		}
	}
}

// AugmentLastUserMessageText appends additional context to the user's input before LLM processing.
// Text is appended with "\n\n" separator to maintain proper formatting for the LLM.
// Preserves the original message's media and metadata.
func (c *request[O]) AugmentLastUserMessageText(text string) {
	c.AugmentLastUserMessage(func(userMessage *messages.UserMessage) *messages.UserMessage {
		return messages.NewUserMessage(userMessage.Text()+"\n\n"+text, userMessage.Media(), userMessage.Metadata())
	})
}

// AugmentLastSystemMessage modifies the most recent system message that guides LLM behavior.
// System messages define the LLM's role, personality, and operational constraints.
// If the function returns nil, the original message is preserved.
func (c *request[O]) AugmentLastSystemMessage(fn func(*messages.SystemMessage) *messages.SystemMessage) {
	index, message := c.findLastMessageByType(messages.System)
	if index == -1 {
		return
	}
	systemMessage, ok := message.(*messages.SystemMessage)
	if ok {
		systemMessage = fn(systemMessage)
		if systemMessage != nil {
			c.messages[index] = systemMessage
		}
	}
}

// AugmentLastSystemMessageText appends additional instructions to the LLM's system prompt.
// Text is appended with "\n\n" separator to maintain proper formatting for the LLM.
// Useful for dynamically adding behavioral guidelines or context-specific instructions.
func (c *request[O]) AugmentLastSystemMessageText(text string) {
	c.AugmentLastSystemMessage(func(systemMessage *messages.SystemMessage) *messages.SystemMessage {
		return messages.NewSystemMessage(systemMessage.Text()+"\n\n"+text, systemMessage.Metadata())
	})
}
