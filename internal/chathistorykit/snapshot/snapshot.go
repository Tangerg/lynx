// Package snapshot deep-copies caller-owned chat protocol values at history
// ownership boundaries.
package snapshot

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// Messages validates and deep-copies messages.
func Messages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for i := range messages {
		if err := messages[i].Validate(); err != nil {
			return nil, fmt.Errorf("chathistory: messages[%d]: %w", i, err)
		}
		cloned[i] = messages[i].Clone()
	}
	return cloned, nil
}

// Request validates and deep-copies a request.
func Request(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return request.Clone(), nil
}

// Message deep-copies a message already known to be valid.
func Message(message chat.Message) chat.Message {
	return message.Clone()
}
