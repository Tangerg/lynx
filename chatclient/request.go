package chatclient

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

func prepareRequest(request *chat.Request, defaults chat.Options) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	prepared := request.Clone()
	merged, err := defaults.Merged(request.Options)
	if err != nil {
		return nil, err
	}
	prepared.Options = merged
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}
