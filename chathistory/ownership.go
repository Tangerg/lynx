package chathistory

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/core/chat"
)

func isNilCapability(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func snapshotMessages(messages []chat.Message) ([]chat.Message, error) {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("chathistory: messages[%d]: %w", index, err)
		}
		cloned[index] = messages[index].Clone()
	}
	return cloned, nil
}

func snapshotRequest(request *chat.Request) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return request.Clone(), nil
}
