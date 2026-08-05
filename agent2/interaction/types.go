package interaction

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

var (
	// ErrInvalidDefinitionConfig reports an incomplete or contradictory
	// Interaction Definition configuration.
	ErrInvalidDefinitionConfig = errors.New("interaction: invalid definition configuration")
	// ErrInvalidDispatcherConfig reports an unusable model or tool binding.
	ErrInvalidDispatcherConfig = errors.New("interaction: invalid dispatcher configuration")
	// ErrInvalidInput reports malformed managed Interaction input.
	ErrInvalidInput = errors.New("interaction: invalid input")
	// ErrInvalidState reports malformed or inconsistent Interaction state.
	ErrInvalidState = errors.New("interaction: invalid execution state")
)

// Input is the complete caller-supplied starting working context. Tools are
// deliberately absent: a Deployment freezes executable tools in its
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

// Output is the final semantic Interaction result. Response is accumulated
// independently of best-effort stream Delta delivery, so it remains complete
// after observer loss or snapshot restoration.
type Output struct {
	// Response is the authoritative accumulated final model response.
	Response chat.Response `json:"response"`

	// ModelCalls is the number of model Effects issued by this Interaction.
	ModelCalls uint32 `json:"model_calls"`
}

// Validate verifies that Output contains a final model response and an actual
// model-call count.
func (output Output) Validate() error {
	if output.ModelCalls == 0 {
		return errors.New("interaction: output model_calls must be positive")
	}
	if err := output.Response.Validate(); err != nil {
		return fmt.Errorf("interaction: output response: %w", err)
	}
	choice := output.Response.First()
	if choice == nil || choice.Message == nil {
		return errors.New("interaction: output response has no final assistant message")
	}
	return nil
}

// DefinitionConfig describes immutable Interaction behavior. MaxModelCalls is
// required because a model-directed loop must have an explicit local stop
// condition in addition to Engine-wide Effect and Step limits.
type DefinitionConfig struct {
	// Name is the stable qualified Definition name.
	Name string

	// Description states the managed behavior for discovery.
	Description string

	// Version is the semantic version of the Definition contract.
	Version string

	// MaxModelCalls bounds model Effects in one Interaction. It must be positive.
	MaxModelCalls uint32
}

// DispatcherConfig binds external capabilities for one Deployment.
type DispatcherConfig struct {
	// Client provides complete and optional streaming model calls.
	Client *chatclient.Client

	// Tools is the frozen model-visible and executable tool manifest.
	Tools []tool.Tool

	// StreamModelResponses selects Client.Stream and publishes each validated
	// response chunk as a best-effort ModelResponseDelta. False uses Client.Call.
	StreamModelResponses bool
}
