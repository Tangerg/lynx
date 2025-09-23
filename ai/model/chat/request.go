package chat

import (
	"errors"
	"sync"

	"github.com/Tangerg/lynx/ai/model"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ model.Request[[]Message, Options] = (*request[Options])(nil)

// Request is a type alias for the most common LLM chat request configuration.
type Request = request[Options]

// NewRequest creates an LLM chat request with conversation messages and optional model parameters.
// Returns an error if no messages are provided, as LLMs require at least one input message.
// If multiple options are given, only the first non-nil option is used.
func NewRequest(msgs []Message, options ...Options) (*Request, error) {
	validMsgs := excludeNilMessages(msgs)
	if len(validMsgs) == 0 {
		return nil, errors.New("at least one valid message is required")
	}

	return &request[Options]{
		messages: validMsgs,
		options:  pkgSlices.FirstOr(options, nil),
		params:   make(map[string]any),
	}, nil
}

// request represents an LLM chat request containing conversation context and model parameters.
type request[O Options] struct {
	messages []Message
	options  O
	mu       sync.RWMutex
	params   map[string]any //context params
}

// Instructions returns the conversation messages that will be sent to the LLM.
func (c *request[O]) Instructions() []Message {
	return c.messages
}

// Options returns the LLM model configuration parameters for this request.
func (c *request[O]) Options() O {
	return c.options
}

func (c *request[O]) Params() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.params
}

func (c *request[O]) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, ok := c.params[key]
	return val, ok
}

func (c *request[O]) Set(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.params[key] = val
}

func (c *request[O]) SetParams(params map[string]any) {
	if params == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.params = params
}

// AugmentLastUserMessageText appends additional context to the user's input before LLM processing.
// Text is appended with "\n\n" separator to maintain proper formatting for the LLM.
// Preserves the original message's media and metadata.
func (c *request[O]) augmentLastUserMessageText(text string) {
	augmentTextLastMessageOfType(c.messages, MessageTypeUser, text)
}
