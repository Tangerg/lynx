package chat

import (
	"errors"
)

// Request is a type alias for the standard LLM chat request
// configuration with default options.
type Request = request[Options]

// NewRequest creates a new chat request with the provided messages.
// Returns an error if the message list is empty or contains only nil values.
func NewRequest(msgs []Message) (*Request, error) {
	validMsgs := excludeNilMessages(msgs)
	if len(validMsgs) == 0 {
		return nil, errors.New("chat request requires at least one valid message")
	}

	return &request[Options]{
		Messages: validMsgs,
		Params:   make(map[string]any),
	}, nil
}

// request represents a chat request containing conversation messages,
// model-specific options, and contextual parameters.
type request[O Options] struct {
	Messages []Message      `json:"messages"`
	Options  O              `json:"options"`
	Params   map[string]any `json:"params"` // context params
}

// ensureExtra initializes the params map if it hasn't been
// created yet to prevent nil pointer operations.
func (r *request[O]) ensureExtra() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

// Get retrieves a parameter value by key.
// Returns the value and true if found, or nil and false otherwise.
func (r *request[O]) Get(key string) (any, bool) {
	r.ensureExtra()
	val, ok := r.Params[key]
	return val, ok
}

// Set stores a parameter value with the specified key.
// Automatically initializes the params map if needed.
func (r *request[O]) Set(key string, val any) {
	r.ensureExtra()
	r.Params[key] = val
}

// augmentLastUserMessageText appends additional text to the last user message
// using "\n\n" as separator while preserving media and metadata.
func (r *request[O]) augmentLastUserMessageText(text string) {
	augmentTextLastMessageOfType(r.Messages, MessageTypeUser, text)
}
