package tool

import (
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

// History manages a collection of messages in chronological order.
// It provides operations for adding, retrieving, and managing
// message history in tool execution contexts.
//
// Note: This type is NOT thread-safe. It is designed for single-threaded use
// within individual tool execution contexts, where each tool maintains its own
// isolated history. If concurrent access is required, external synchronization
// must be provided by the caller.
//
// In typical usage, each tool call gets its own History instance, eliminating
// the need for concurrent access and keeping the implementation simple and efficient.
type History struct {
	messages []messages.Message
}

// NewHistory creates a new empty History instance.
// The returned History is ready to use and initialized with an empty message slice.
func NewHistory() *History {
	return &History{
		messages: make([]messages.Message, 0),
	}
}

// AddMessages appends one or more messages to the history in the order provided.
// This method accepts variadic arguments, allowing for single or batch message additions.
//
// Example:
//
//	history.AddMessages(msg1)
//	history.AddMessages(msg1, msg2, msg3)
func (h *History) AddMessages(msgs ...messages.Message) {
	h.messages = append(h.messages, msgs...)
}

// FirstMessage returns the first message in the history.
// Returns the message and true if the history is not empty,
// otherwise returns nil and false.
func (h *History) FirstMessage() (messages.Message, bool) {
	if h.Size() == 0 {
		return nil, false
	}
	return h.messages[0], true
}

// LastMessage returns the most recent message in the history.
// Returns the message and true if the history is not empty,
// otherwise returns nil and false.
func (h *History) LastMessage() (messages.Message, bool) {
	if h.Size() == 0 {
		return nil, false
	}
	return h.messages[len(h.messages)-1], true
}

// Messages returns a copy of all messages in the history.
// The returned slice is a clone, so modifications to it will not
// affect the original history. This ensures data integrity while
// allowing safe access to the message collection.
func (h *History) Messages() []messages.Message {
	return slices.Clone(h.messages)
}

// Size returns the total number of messages currently stored in the history.
func (h *History) Size() int {
	return len(h.messages)
}

// Clear removes all messages from the history, resetting it to an empty state.
// After calling Clear(), Size() will return 0 and Messages() will return an empty slice.
// The underlying array capacity is preserved for efficient reuse.
func (h *History) Clear() {
	h.messages = h.messages[:0]
}
