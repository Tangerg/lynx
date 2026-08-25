package storage

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// MessageCodec owns the canonical persisted representation of chat messages.
// Its zero value is ready to use.
type MessageCodec struct{}

// Encode serializes messages in order.
func (MessageCodec) Encode(messages []chat.Message) ([][]byte, error) {
	encoded := make([][]byte, len(messages))
	for index := range messages {
		raw, err := messages[index].MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", index, err)
		}
		encoded[index] = raw
	}
	return encoded, nil
}

// Decode restores one canonical persisted message.
func (MessageCodec) Decode(raw []byte) (chat.Message, error) {
	var message chat.Message
	if err := message.UnmarshalJSON(raw); err != nil {
		return chat.Message{}, err
	}
	return message, nil
}
