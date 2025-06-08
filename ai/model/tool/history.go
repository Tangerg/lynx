package tool

import (
	"iter"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

// History defines the interface for managing a collection of messages in chronological order.
// It provides operations for adding, retrieving, and managing message history in tool execution contexts.
//
// Note: Implementations are NOT required to be thread-safe. They are designed for single-threaded use
// within individual tool execution contexts, where each tool maintains its own isolated history.
// If concurrent access is required, external synchronization must be provided by the caller.
//
// In typical usage, each tool call gets its own History instance, eliminating
// the need for concurrent access and keeping the implementation simple and efficient.
type History interface {
	// Add appends one or more messages to the history in the order provided.
	// This method accepts variadic arguments, allowing for single or batch message additions.
	//
	// Example:
	//	history.Add(msg1)
	//	history.Add(msg1, msg2, msg3)
	Add(msgs ...messages.Message)

	// First returns the first message in the history.
	// Returns the message and true if the history is not empty,
	// otherwise returns nil and false.
	//
	// This method is useful for accessing the initial context or starting point
	// of a conversation or tool execution sequence.
	First() (messages.Message, bool)

	// Last returns the most recent message in the history.
	// Returns the message and true if the history is not empty,
	// otherwise returns nil and false.
	//
	// This method is commonly used to get the latest context or most recent
	// interaction in the message history.
	Last() (messages.Message, bool)

	// All returns a copy of all messages in the history.
	// The returned slice is a clone, so modifications to it will not
	// affect the original history. This ensures data integrity while
	// allowing safe access to the message collection.
	//
	// Use this method when you need to process all messages or pass
	// the complete history to other components.
	All() []messages.Message

	// Iter returns an iterator over all messages in the history.
	// This allows for range-over-func iteration in Go 1.23+.
	//
	// Example:
	//	for msg := range history.Iter() {
	//	    // process message
	//	}
	//
	// This method is memory-efficient for processing large histories
	// as it doesn't create a copy of the entire slice.
	Iter() iter.Seq[messages.Message]

	// Size returns the total number of messages currently stored in the history.
	//
	// This method is useful for checking if the history is empty,
	// implementing pagination, or monitoring history growth.
	Size() int

	// Clear removes all messages from the history, resetting it to an empty state.
	// After calling Clear(), Size() will return 0 and All() will return an empty slice.
	//
	// This method is typically used when starting a new conversation context
	// or when memory cleanup is needed. The implementation may preserve
	// underlying array capacity for efficient reuse.
	Clear()

	// Clone creates a deep copy of the History instance.
	// Returns a new History with a cloned messages slice, ensuring that modifications
	// to the clone do not affect the original History and vice versa.
	//
	// This method is useful for creating snapshots of the current state,
	// implementing undo/redo functionality, or branching conversation flows.
	Clone() History
}

// history is the internal implementation of the History interface.
type history struct {
	messages []messages.Message
}

func NewHistory() History {
	return &history{
		messages: make([]messages.Message, 0),
	}
}

func (h *history) Add(msgs ...messages.Message) {
	h.messages = append(h.messages, msgs...)
}

func (h *history) First() (messages.Message, bool) {
	if h.Size() == 0 {
		return nil, false
	}
	return h.messages[0], true
}

func (h *history) Last() (messages.Message, bool) {
	if h.Size() == 0 {
		return nil, false
	}
	return h.messages[len(h.messages)-1], true
}

func (h *history) All() []messages.Message {
	return slices.Clone(h.messages)
}

func (h *history) Iter() iter.Seq[messages.Message] {
	return slices.Values(h.messages)
}

func (h *history) Size() int {
	return len(h.messages)
}

func (h *history) Clear() {
	h.messages = h.messages[:0]
}

func (h *history) Clone() History {
	return &history{
		messages: slices.Clone(h.messages),
	}
}
