package redis

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/samber/lo"
)

func encodeMessages(messages []chat.Message) ([][]byte, error) {
	return lo.MapErr(messages, func(message chat.Message, index int) ([]byte, error) {
		raw, err := message.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", index, err)
		}
		return raw, nil
	})
}

func decodeMessage(raw []byte) (chat.Message, error) {
	var message chat.Message
	if err := message.UnmarshalJSON(raw); err != nil {
		return chat.Message{}, err
	}
	return message, nil
}
