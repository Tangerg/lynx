// Package conversation owns the model-context message sequence independently
// from Runs, transcript observations, and executor working state.
package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

var (
	// ErrInvalid reports a malformed conversation value.
	ErrInvalid = errors.New("conversation: invalid")
	// ErrNotEmpty reports an attempt to seed a conversation that already has
	// messages. Seed is a fork/import operation, never append under another name.
	ErrNotEmpty = errors.New("conversation: seed requires an empty conversation")
)

// Conversation is an ownership-isolated model-context sequence. It contains no
// persistence, Run, transcript, or executor state.
type Conversation struct {
	messages []chat.Message
}

// New validates and owns messages as one conversation value.
func New(messages []chat.Message) (Conversation, error) {
	owned := make([]chat.Message, len(messages))
	for index, message := range messages {
		if err := message.Validate(); err != nil {
			return Conversation{}, fmt.Errorf("%w: message[%d]: %w", ErrInvalid, index, err)
		}
		owned[index] = message.Clone()
	}
	return Conversation{messages: owned}, nil
}

// Messages returns an ownership-isolated snapshot in model-context order.
func (c Conversation) Messages() []chat.Message {
	out := make([]chat.Message, len(c.messages))
	for index, message := range c.messages {
		out[index] = message.Clone()
	}
	return out
}

// Count returns the current message watermark.
func (c Conversation) Count() int { return len(c.messages) }

// Seed replaces an empty conversation with a validated fork/import prefix.
func (c Conversation) Seed(messages []chat.Message) (Conversation, error) {
	if len(c.messages) != 0 {
		return Conversation{}, ErrNotEmpty
	}
	return New(messages)
}

// Truncate returns the prefix ending at keepN. Values below zero clear the
// conversation; values beyond the current watermark are a no-op.
func (c Conversation) Truncate(keepN int) Conversation {
	keepN = max(keepN, 0)
	keepN = min(keepN, len(c.messages))
	truncated, _ := New(c.messages[:keepN])
	return truncated
}
