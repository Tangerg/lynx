// Package codec owns the current durable history wire boundary shared by
// every chathistory backend.
package codec

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// EncodeMessage validates and writes the current core/chat tagged wire.
func EncodeMessage(message chat.Message) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, fmt.Errorf("chathistory codec: encode: %w", err)
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("chathistory codec: encode: %w", err)
	}
	return raw, nil
}

// DecodeMessage decodes the current role-tagged core/chat wire. Historical
// type-tagged formats are deliberately unsupported; callers must migrate data
// before upgrading.
func DecodeMessage(raw []byte) (chat.Message, error) {
	var message chat.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return chat.Message{}, fmt.Errorf("chathistory codec: decode: %w", err)
	}
	return message, nil
}
