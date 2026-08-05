package agent2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidEffect = errors.New("agent: invalid effect")

// EffectTarget identifies which closed execution boundary owns an Effect.
// Framework Effects are interpreted by the Engine; Dispatcher Effects remain
// opaque to the Engine and are interpreted by the Deployment-bound dispatcher.
type EffectTarget uint8

const (
	EffectTargetInvalid EffectTarget = iota
	EffectTargetFramework
	EffectTargetDispatcher
)

func (target EffectTarget) String() string {
	switch target {
	case EffectTargetFramework:
		return "framework"
	case EffectTargetDispatcher:
		return "dispatcher"
	default:
		return "invalid"
	}
}

func parseEffectTarget(value string) (EffectTarget, error) {
	switch value {
	case "framework":
		return EffectTargetFramework, nil
	case "dispatcher":
		return EffectTargetDispatcher, nil
	default:
		return EffectTargetInvalid, fmt.Errorf("%w: unknown target %q", ErrInvalidEffect, value)
	}
}

// Effect is an immutable request for an operation outside Execution.Step.
// Payload is frozen before dispatch and interpreted only by Target's owner.
// EffectID is deliberately absent because the Engine assigns it during prepare.
type Effect struct {
	target  EffectTarget
	payload json.RawMessage
}

// NewDispatcherEffect creates a Strategy-owned Effect. The Engine may copy,
// order, identify, and route payload, but must not inspect it.
func NewDispatcherEffect(payload json.RawMessage) (Effect, error) {
	return newEffect(EffectTargetDispatcher, payload)
}

func newEffect(target EffectTarget, payload json.RawMessage) (Effect, error) {
	if target != EffectTargetFramework && target != EffectTargetDispatcher {
		return Effect{}, fmt.Errorf("%w: invalid target", ErrInvalidEffect)
	}
	normalized, err := normalizeJSON(payload, maxWireBytes)
	if err != nil {
		return Effect{}, fmt.Errorf("%w: payload: %w", ErrInvalidEffect, err)
	}
	return Effect{target: target, payload: normalized}, nil
}

// Target returns the owner responsible for interpreting Payload.
func (e Effect) Target() EffectTarget { return e.target }

// Payload returns an independently owned copy of the operation intent.
func (e Effect) Payload() json.RawMessage { return bytes.Clone(e.payload) }

// Valid reports whether the Effect was created through a validated boundary.
func (e Effect) Valid() bool {
	return (e.target == EffectTargetFramework || e.target == EffectTargetDispatcher) && len(e.payload) > 0
}

func (e Effect) clone() Effect {
	return Effect{target: e.target, payload: bytes.Clone(e.payload)}
}

func (e Effect) MarshalJSON() ([]byte, error) {
	if !e.Valid() {
		return nil, ErrInvalidEffect
	}
	return json.Marshal(effectWire{Target: e.target.String(), Payload: e.payload})
}

func (e *Effect) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidEffect)
	}
	var wire effectWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidEffect, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEffect, err)
	}
	target, err := parseEffectTarget(wire.Target)
	if err != nil {
		return err
	}
	value, err := newEffect(target, wire.Payload)
	if err != nil {
		return err
	}
	*e = value
	return nil
}

type effectWire struct {
	Target  string          `json:"target"`
	Payload json.RawMessage `json:"payload"`
}
