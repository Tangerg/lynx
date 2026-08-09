package interaction

import (
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// Input is the complete caller-supplied starting working context. Tools are
// deliberately absent: a Deployment freezes executable Tools in its
// Dispatcher so model-visible definitions and executable behavior cannot drift
// per Process.
type Input struct {
	// Messages is the initial provider-neutral WorkingContext.
	Messages []chat.Message `json:"messages"`

	// Options contains request-specific generation overrides.
	Options chat.Options `json:"options,omitzero"`
}

// Validate verifies the provider-neutral model request represented by Input.
func (input Input) Validate() error {
	request := &chat.Request{Messages: input.Messages, Options: input.Options}
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return nil
}
