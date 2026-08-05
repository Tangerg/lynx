package agent2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const childProtocolSchemaVersion uint16 = 1

var ErrInvalidChild = errors.New("agent: invalid child process request")

// DeploymentResolver supplies an exact immutable Deployment for a child
// Process. The Engine accepts only a result whose reference exactly matches the
// requested reference. Implementations must be safe for concurrent use and
// must not re-enter the requesting Process.
type DeploymentResolver interface {
	Resolve(context.Context, DeploymentRef) (Deployment, error)
}

// ChildSpec is the complete Strategy-declared intent for one child Process.
// Input is validated by the target Deployment before any Process is created.
type ChildSpec struct {
	Key          ChildKey      `json:"key"`
	Deployment   DeploymentRef `json:"deployment"`
	Input        Input         `json:"input"`
	Budget       Budget        `json:"budget"`
	Capabilities CapabilitySet `json:"capabilities"`
}

// Valid reports whether the child intent contains stable identity, exact
// deployment identity, and portable input.
func (spec ChildSpec) Valid() bool {
	return spec.Key.Valid() && spec.Deployment.Valid() && spec.Input.Valid() &&
		spec.Budget.Valid() && spec.Capabilities.Valid()
}

// StartChild creates a Framework-owned Effect requesting one independently
// managed child Process. The Engine derives the child ProcessID; Execution code
// cannot construct or start the Process directly.
func StartChild(spec ChildSpec) (Effect, error) {
	if !spec.Valid() {
		return Effect{}, ErrInvalidChild
	}
	payload, err := json.Marshal(childStartEffectWire{
		Operation: frameworkEffectStartChild, SchemaVersion: frameworkEffectSchemaVersion,
		Spec: spec,
	})
	if err != nil {
		return Effect{}, fmt.Errorf("%w: encode start request: %v", ErrInvalidChild, err)
	}
	return newEffect(EffectTargetFramework, payload)
}

// ChildStartResult is the definite result of one StartChild Effect. Success
// contains the Engine-created child ProcessID; failure contains a stable
// Framework Failure and never masquerades as an unknown external outcome.
type ChildStartResult struct {
	key        ChildKey
	processID  ProcessID
	deployment DeploymentRef
	failure    Failure
}

// Key returns the logical child identity declared by the Execution.
func (result ChildStartResult) Key() ChildKey { return result.key }

// ProcessID returns the created child identity and true on success.
func (result ChildStartResult) ProcessID() (ProcessID, bool) {
	return result.processID, result.processID.Valid() && !result.failure.Valid()
}

// DeploymentRef returns the exact child execution binding.
func (result ChildStartResult) DeploymentRef() DeploymentRef { return result.deployment }

// Failure returns the definite start failure and true when no child was
// created.
func (result ChildStartResult) Failure() (Failure, bool) {
	return result.failure, result.failure.Valid() && !result.processID.Valid()
}

// Valid reports whether result contains exactly one success or failure.
func (result ChildStartResult) Valid() bool {
	return result.key.Valid() && result.deployment.Valid() &&
		(result.processID.Valid() != result.failure.Valid())
}

// ParseChildStartResult decodes a Framework-owned child-start settlement
// Signal. The Signal must not address a wait.
func ParseChildStartResult(signal Signal) (ChildStartResult, error) {
	if !signal.Valid() {
		return ChildStartResult{}, ErrInvalidSignal
	}
	if _, addressed := signal.WaitID(); addressed {
		return ChildStartResult{}, fmt.Errorf("%w: child-start Signal addresses a wait", ErrInvalidChild)
	}
	return decodeChildStartResult(signal.Payload())
}

type childStartEffectWire struct {
	Operation     string    `json:"operation"`
	SchemaVersion uint16    `json:"schema_version"`
	Spec          ChildSpec `json:"spec"`
}

type childStartResultWire struct {
	SchemaVersion uint16        `json:"schema_version"`
	Operation     string        `json:"operation"`
	Key           ChildKey      `json:"key"`
	ProcessID     *ProcessID    `json:"process_id,omitempty"`
	Deployment    DeploymentRef `json:"deployment"`
	Failure       *Failure      `json:"failure,omitempty"`
}

func decodeChildStartEffect(payload json.RawMessage) (ChildSpec, error) {
	var wire childStartEffectWire
	if err := decodeStrictJSON(payload, &wire); err != nil {
		return ChildSpec{}, fmt.Errorf("%w: decode start request: %v", ErrInvalidChild, err)
	}
	if wire.Operation != frameworkEffectStartChild ||
		wire.SchemaVersion != frameworkEffectSchemaVersion || !wire.Spec.Valid() {
		return ChildSpec{}, ErrInvalidChild
	}
	return wire.Spec, nil
}

func encodeChildStartResult(result ChildStartResult) (json.RawMessage, error) {
	if !result.Valid() {
		return nil, ErrInvalidChild
	}
	wire := childStartResultWire{
		SchemaVersion: childProtocolSchemaVersion,
		Operation:     frameworkEffectStartChild,
		Key:           result.key,
		Deployment:    result.deployment,
	}
	if result.processID.Valid() {
		processID := result.processID
		wire.ProcessID = &processID
	} else {
		failure := result.failure
		wire.Failure = &failure
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("%w: encode start result: %v", ErrInvalidChild, err)
	}
	return payload, nil
}

func decodeChildStartResult(payload json.RawMessage) (ChildStartResult, error) {
	var wire childStartResultWire
	if err := decodeStrictJSON(payload, &wire); err != nil {
		return ChildStartResult{}, fmt.Errorf("%w: decode start result: %v", ErrInvalidChild, err)
	}
	var processID ProcessID
	if wire.ProcessID != nil {
		processID = *wire.ProcessID
	}
	var failure Failure
	if wire.Failure != nil {
		failure = *wire.Failure
	}
	result := ChildStartResult{
		key: wire.Key, processID: processID, deployment: wire.Deployment, failure: failure,
	}
	if wire.SchemaVersion != childProtocolSchemaVersion ||
		wire.Operation != frameworkEffectStartChild || !result.Valid() {
		return ChildStartResult{}, ErrInvalidChild
	}
	return result, nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}
