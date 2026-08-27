package interaction

import (
	"errors"
	"fmt"
	"slices"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
)

// ErrInvalidSteer reports an empty or non-user steering message set.
var ErrInvalidSteer = errors.New("interaction: invalid steer")

type steerBatch struct {
	Messages  []chat.Message   `json:"messages"`
	SignalIDs []agent.SignalID `json:"signal_ids"`
}

func (s steerBatch) empty() bool {
	return len(s.Messages) == 0 && len(s.SignalIDs) == 0
}

func (s steerBatch) validate() error {
	if err := validateSteeringMessages(s.Messages); err != nil {
		return err
	}
	return validateSteerSignalIDs(s.SignalIDs)
}

func (s steerBatch) clone() steerBatch {
	return steerBatch{
		Messages: cloneMessages(s.Messages), SignalIDs: slices.Clone(s.SignalIDs),
	}
}

func (s *steerBatch) appendSignal(signal agent.Signal, messages []chat.Message) error {
	if s == nil || !signal.ID().Valid() {
		return ErrInvalidSteer
	}
	if _, addressed := signal.WaitID(); addressed {
		return fmt.Errorf("%w: steer Signal must not address a wait", ErrInvalidSteer)
	}
	s.Messages = append(s.Messages, cloneMessages(messages)...)
	s.SignalIDs = append(s.SignalIDs, signal.ID())
	return nil
}

func validateSteerSignalIDs(ids []agent.SignalID) error {
	if len(ids) == 0 {
		return fmt.Errorf("%w: at least one SignalID is required", ErrInvalidSteer)
	}
	seen := make(map[agent.SignalID]struct{}, len(ids))
	for index, id := range ids {
		if !id.Valid() {
			return fmt.Errorf("%w: SignalID %d is invalid", ErrInvalidSteer, index)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: duplicate SignalID %q", ErrInvalidSteer, id.String())
		}
		seen[id] = struct{}{}
	}
	return nil
}

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
