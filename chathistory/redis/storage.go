package redis

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/core/chat"
)

func isNilCapability(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() || (reflected.Kind() == reflect.Pointer && reflected.IsNil())
}

func encodeMessages(messages []chat.Message) ([][]byte, error) {
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

func decodeMessage(raw []byte) (chat.Message, error) {
	var message chat.Message
	if err := message.UnmarshalJSON(raw); err != nil {
		return chat.Message{}, err
	}
	return message, nil
}
