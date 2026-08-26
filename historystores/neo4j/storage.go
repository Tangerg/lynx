package neo4j

import (
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/Tangerg/lynx/core/chat"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

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

type sequenceGenerator struct {
	mu   sync.Mutex
	last int64
}

func (s *sequenceGenerator) Reserve(count int) int64 {
	return s.reserveAt(time.Now().UnixNano(), count)
}

func (s *sequenceGenerator) reserveAt(candidate int64, count int) int64 {
	if count <= 0 {
		panic("neo4j: sequence count must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	first := candidate
	if first <= s.last {
		first = s.last + 1
	}
	s.last = first + int64(count) - 1
	return first
}
