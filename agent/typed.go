package agent

import (
	"errors"
	"fmt"
)

// ErrInvalidTypedAdapter reports a nil or malformed erased Definition binding.
var ErrInvalidTypedAdapter = errors.New("agent: invalid typed adapter")

// Typed provides Go type ergonomics at a Definition boundary while preserving
// the non-generic Definition narrow waist used by Engine and catalogs.
type Typed[I, O any] struct {
	definition Definition
}

// NewTyped validates and wraps one erased Definition. Values are still checked
// against the Descriptor schema on every typed encode and decode; the adapter
// does not claim that arbitrary Go types are schema-equivalent in advance.
func NewTyped[I, O any](definition Definition) (Typed[I, O], error) {
	if nilInterface(definition) {
		return Typed[I, O]{}, fmt.Errorf("%w: definition is required", ErrInvalidTypedAdapter)
	}
	if !definition.Descriptor().Valid() {
		return Typed[I, O]{}, fmt.Errorf("%w: %w", ErrInvalidTypedAdapter, ErrInvalidDescriptor)
	}
	return Typed[I, O]{definition: definition}, nil
}

// Definition returns the erased Definition for heterogeneous Engine storage.
func (adapter Typed[I, O]) Definition() Definition { return adapter.definition }

// EncodeInput converts a Go value to Input and validates the authoritative
// Descriptor input schema before the value reaches Definition.Start.
func (adapter Typed[I, O]) EncodeInput(value I) (Input, error) {
	if adapter.definition == nil {
		return Input{}, ErrInvalidTypedAdapter
	}
	input, err := EncodeInput(value)
	if err != nil {
		return Input{}, err
	}
	if err := adapter.definition.Descriptor().ValidateInput(input); err != nil {
		return Input{}, fmt.Errorf("%w: input: %w", ErrInvalidTypedAdapter, err)
	}
	return input, nil
}

// DecodeOutput validates the authoritative Descriptor output schema before
// strictly decoding the final semantic result into O.
func (adapter Typed[I, O]) DecodeOutput(output Output) (O, error) {
	var zero O
	if adapter.definition == nil {
		return zero, ErrInvalidTypedAdapter
	}
	if err := adapter.definition.Descriptor().ValidateOutput(output); err != nil {
		return zero, fmt.Errorf("%w: output: %w", ErrInvalidTypedAdapter, err)
	}
	value, err := DecodeOutput[O](output)
	if err != nil {
		return zero, fmt.Errorf("%w: output: %w", ErrInvalidTypedAdapter, err)
	}
	return value, nil
}
