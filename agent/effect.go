package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidEffect reports a malformed or contradictory Effect intent.
var ErrInvalidEffect = errors.New("agent: invalid effect")

// EffectTarget identifies which of the two execution boundaries owns an Effect.
// Framework Effects are interpreted by the Engine; Dispatcher Effects remain
// opaque to the Engine and are interpreted by the Deployment-bound dispatcher.
type EffectTarget uint8

const (
	// EffectTargetInvalid is the invalid zero value.
	EffectTargetInvalid EffectTarget = iota
	// EffectTargetFramework identifies an Engine-interpreted Effect.
	EffectTargetFramework
	// EffectTargetDispatcher identifies a Strategy dispatcher Effect.
	EffectTargetDispatcher
)

// String returns the stable Effect target name.
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
	target       EffectTarget
	payload      json.RawMessage
	requirements CapabilitySet
}

// NewDispatcherEffect creates a Strategy-owned Effect. The Engine may copy,
// order, identify, and route payload, but must not inspect it.
func NewDispatcherEffect(payload json.RawMessage, required ...Capability) (Effect, error) {
	requirements, err := NewCapabilitySet(required...)
	if err != nil {
		return Effect{}, fmt.Errorf("%w: required capabilities: %w", ErrInvalidEffect, err)
	}
	return newEffectWithCapabilities(EffectTargetDispatcher, payload, requirements)
}

// RequestWait creates the Framework Effect that asks the Engine to mint one
// WaitID for key. signalPayload remains Strategy-owned and is returned unchanged
// in the internal Signal that carries the minted WaitID back to the Execution.
func RequestWait(key WaitKey, signalPayload json.RawMessage) (Effect, error) {
	if !key.Valid() {
		return Effect{}, fmt.Errorf("%w: wait key: %w", ErrInvalidEffect, ErrInvalidIdentity)
	}
	normalized, err := normalizeJSON(signalPayload, maxWireBytes)
	if err != nil {
		return Effect{}, fmt.Errorf("%w: wait signal payload: %w", ErrInvalidEffect, err)
	}
	payload, err := json.Marshal(waitRequestWire{
		Operation:     frameworkEffectWait,
		SchemaVersion: frameworkEffectSchemaVersion,
		Key:           key,
		SignalPayload: normalized,
	})
	if err != nil {
		return Effect{}, fmt.Errorf("%w: encode wait request: %w", ErrInvalidEffect, err)
	}
	return newEffect(EffectTargetFramework, payload)
}

func newEffect(target EffectTarget, payload json.RawMessage) (Effect, error) {
	return newEffectWithCapabilities(target, payload, CapabilitySet{})
}

func newEffectWithCapabilities(
	target EffectTarget,
	payload json.RawMessage,
	requirements CapabilitySet,
) (Effect, error) {
	if target != EffectTargetFramework && target != EffectTargetDispatcher {
		return Effect{}, fmt.Errorf("%w: invalid target", ErrInvalidEffect)
	}
	if !requirements.Valid() || target == EffectTargetFramework && len(requirements.values) != 0 {
		return Effect{}, fmt.Errorf("%w: invalid required capabilities", ErrInvalidEffect)
	}
	normalized, err := normalizeJSON(payload, maxWireBytes)
	if err != nil {
		return Effect{}, fmt.Errorf("%w: payload: %w", ErrInvalidEffect, err)
	}
	if target == EffectTargetFramework {
		if err := validateFrameworkEffectPayload(normalized); err != nil {
			return Effect{}, err
		}
	}
	return Effect{target: target, payload: normalized, requirements: requirements}, nil
}

// Target returns the owner responsible for interpreting Payload.
func (e Effect) Target() EffectTarget { return e.target }

// Payload returns an independently owned copy of the operation intent.
func (e Effect) Payload() json.RawMessage { return bytes.Clone(e.payload) }

// RequiredCapabilities returns the immutable authority set the Process must
// possess before this Dispatcher Effect may be prepared.
func (e Effect) RequiredCapabilities() CapabilitySet { return e.requirements }

// Valid reports whether the Effect was created through a validated boundary.
func (e Effect) Valid() bool {
	return (e.target == EffectTargetFramework || e.target == EffectTargetDispatcher) &&
		len(e.payload) > 0 && e.requirements.Valid() &&
		(e.target != EffectTargetFramework || len(e.requirements.values) == 0)
}

func (e Effect) clone() Effect {
	requirements, _ := NewCapabilitySet(e.requirements.values...)
	return Effect{target: e.target, payload: bytes.Clone(e.payload), requirements: requirements}
}

// MarshalJSON returns the validated immutable Effect intent.
func (e Effect) MarshalJSON() ([]byte, error) {
	if !e.Valid() {
		return nil, ErrInvalidEffect
	}
	return json.Marshal(effectWire{
		Target: e.target.String(), Payload: e.payload,
		RequiredCapabilities: e.requirements.Values(),
	})
}

// UnmarshalJSON replaces e with a strictly decoded Effect.
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
	requirements, err := NewCapabilitySet(wire.RequiredCapabilities...)
	if err != nil {
		return fmt.Errorf("%w: required capabilities: %w", ErrInvalidEffect, err)
	}
	value, err := newEffectWithCapabilities(target, wire.Payload, requirements)
	if err != nil {
		return err
	}
	*e = value
	return nil
}

type effectWire struct {
	Target               string          `json:"target"`
	Payload              json.RawMessage `json:"payload"`
	RequiredCapabilities []Capability    `json:"required_capabilities,omitempty"`
}

const (
	frameworkEffectSchemaVersion = 2
	frameworkEffectWait          = "wait"
	frameworkEffectStartChild    = "start_child"
	frameworkEffectWaitChildren  = "wait_children"
)

type waitRequestWire struct {
	Operation     string          `json:"operation"`
	SchemaVersion uint16          `json:"schema_version"`
	Key           WaitKey         `json:"key"`
	SignalPayload json.RawMessage `json:"signal_payload"`
}

func decodeWaitRequest(effect Effect) (WaitKey, json.RawMessage, error) {
	if effect.Target() != EffectTargetFramework {
		return WaitKey{}, nil, fmt.Errorf("%w: Effect is not framework-owned", ErrInvalidEffect)
	}
	return decodeWaitRequestPayload(effect.payload)
}

func decodeWaitRequestPayload(payload json.RawMessage) (WaitKey, json.RawMessage, error) {
	var wire waitRequestWire
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return WaitKey{}, nil, fmt.Errorf("%w: decode Framework Effect: %w", ErrInvalidEffect, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return WaitKey{}, nil, fmt.Errorf("%w: framework Effect: %w", ErrInvalidEffect, err)
	}
	if wire.Operation != frameworkEffectWait || wire.SchemaVersion != frameworkEffectSchemaVersion || !wire.Key.Valid() {
		return WaitKey{}, nil, fmt.Errorf("%w: unsupported Framework Effect", ErrInvalidEffect)
	}
	normalized, err := normalizeJSON(wire.SignalPayload, maxWireBytes)
	if err != nil {
		return WaitKey{}, nil, fmt.Errorf("%w: framework Effect signal payload: %w", ErrInvalidEffect, err)
	}
	return wire.Key, normalized, nil
}

func validateFrameworkEffectPayload(payload json.RawMessage) error {
	operation, err := frameworkEffectOperation(payload)
	if err != nil {
		return err
	}
	switch operation {
	case frameworkEffectWait:
		_, _, err := decodeWaitRequestPayload(payload)
		return err
	case frameworkEffectStartChild:
		_, err := decodeChildStartEffect(payload)
		return err
	case frameworkEffectWaitChildren:
		_, err := decodeChildWaitEffect(payload)
		return err
	default:
		return fmt.Errorf("%w: unsupported Framework Effect", ErrInvalidEffect)
	}
}

func frameworkEffectOperation(payload json.RawMessage) (string, error) {
	var header struct {
		Operation     string `json:"operation"`
		SchemaVersion uint16 `json:"schema_version"`
	}
	if err := json.Unmarshal(payload, &header); err != nil {
		return "", fmt.Errorf("%w: decode Framework Effect header: %w", ErrInvalidEffect, err)
	}
	if header.SchemaVersion != frameworkEffectSchemaVersion {
		return "", fmt.Errorf("%w: unsupported Framework Effect schema", ErrInvalidEffect)
	}
	return header.Operation, nil
}
