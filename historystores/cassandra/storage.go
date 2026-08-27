package cassandra

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Cassandra TIMEUUID timestamps advance in 100-nanosecond ticks. Reserving in
// that unit prevents distinct message positions from collapsing to one UUID.
const timeUUIDTick = 100 * time.Nanosecond

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

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

type sequenceGenerator struct {
	mu   sync.Mutex
	last int64
}

func (s *sequenceGenerator) reserveTimeUUIDs(count int) int64 {
	if count <= 0 {
		panic("cassandra: sequence count must be positive")
	}
	span := int64(count) * int64(timeUUIDTick)
	candidate := time.Now().UnixNano()
	s.mu.Lock()
	defer s.mu.Unlock()
	first := candidate
	if first <= s.last {
		first = s.last + 1
	}
	s.last = first + span - 1
	return first
}
