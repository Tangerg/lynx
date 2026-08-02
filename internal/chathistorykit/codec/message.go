// Package codec owns the current durable history wire boundary shared by
// every chathistory backend.
package codec

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// EncodeMessage validates and writes the current core/chat tagged wire.
func EncodeMessage(message chat.Message) ([]byte, error) {
	raw, err := message.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encode current message wire: %w", err)
	}
	return raw, nil
}

// EncodeMessages validates and encodes a batch while preserving its order.
func EncodeMessages(messages []chat.Message) ([][]byte, error) {
	encoded := make([][]byte, len(messages))
	for index, message := range messages {
		raw, err := EncodeMessage(message)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", index, err)
		}
		encoded[index] = raw
	}
	return encoded, nil
}

// DecodeMessage decodes the current role-tagged core/chat wire. Historical
// type-tagged formats are deliberately unsupported; callers must migrate data
// before upgrading.
func DecodeMessage(raw []byte) (chat.Message, error) {
	var message chat.Message
	if err := message.UnmarshalJSON(raw); err != nil {
		return chat.Message{}, fmt.Errorf("decode current message wire: %w", err)
	}
	return message, nil
}
