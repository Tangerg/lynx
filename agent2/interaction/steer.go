package interaction

import (
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

// ErrInvalidSteer reports an empty or non-user steering message set.
var ErrInvalidSteer = errors.New("interaction: invalid steer")

// NewSteerSignal constructs one deduplicated, model-visible steering input.
// The Signal never interrupts an in-flight operation. When accepted during a
// model call, Tool segment, or Delegate start phase, its earliest visible
// boundary is the next model request after that entire ToolCall batch settles.
// A Process already Waiting for Tool input or child completion rejects
// unaddressed steering. Worst-case accepted-steer latency is therefore the
// remaining duration of the active operation plus Engine Step scheduling time.
func NewSteerSignal(id agent.SignalID, messages ...chat.Message) (agent.SignalRequest, error) {
	if err := validateSteeringMessages(messages); err != nil {
		return agent.SignalRequest{}, err
	}
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationSteer,
		Steer:         &steerInput{Messages: cloneMessages(messages)},
	})
	if err != nil {
		return agent.SignalRequest{}, err
	}
	return agent.NewSignalRequest(id, agent.WaitID{}, payload)
}

func validateSteeringMessages(messages []chat.Message) error {
	if len(messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidSteer)
	}
	for index := range messages {
		message := messages[index]
		if err := message.Validate(); err != nil {
			return fmt.Errorf("%w: message %d: %w", ErrInvalidSteer, index, err)
		}
		if message.Role != chat.RoleUser {
			return fmt.Errorf("%w: message %d must have user role", ErrInvalidSteer, index)
		}
	}
	return nil
}
