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
	prepared.Options = mergeOptions(defaults, request.Options)
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

// mergeOptions applies per-request overrides over client defaults. The
// field-aware merge lives on chat.Options.Overlay so it can't drift from the
// option set core owns.
func mergeOptions(defaults, overrides chat.Options) chat.Options {
	return defaults.Overlay(overrides)
}
