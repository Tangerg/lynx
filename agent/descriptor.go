package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	maxDescriptionBytes = 4096
)

// ErrInvalidDescriptor reports a malformed static Definition contract.
var ErrInvalidDescriptor = errors.New("agent: invalid descriptor")

// DescriptorConfig contains the complete static contract of a Definition.
// Executable implementation and frozen configuration identity belong to a
// Deployment, not this contract.
type DescriptorConfig struct {
	// Name is a stable lowercase qualified Definition name.
	Name string

	// Description states the Definition's behavior for human and model-facing
	// discovery without execution-specific state.
	Description string

	// Version is a canonical MAJOR.MINOR.PATCH semantic version.
	Version string

	// InputSchema is the authoritative structural contract for Process input.
	InputSchema Schema

	// OutputSchema is the authoritative structural contract for completed output.
	OutputSchema Schema
}

// Descriptor is an immutable Definition contract. It contains no executable
// behavior or Deployment configuration.
type Descriptor struct {
	name         string
	description  string
	version      string
	inputSchema  Schema
	outputSchema Schema
	digest       Digest
}

// NewDescriptor validates and takes an immutable snapshot of config.
func NewDescriptor(config DescriptorConfig) (Descriptor, error) {
	if err := validateDescriptorConfig(config); err != nil {
		return Descriptor{}, err
	}
	descriptor := Descriptor{
		name:         config.Name,
		description:  config.Description,
		version:      config.Version,
		inputSchema:  config.InputSchema.clone(),
		outputSchema: config.OutputSchema.clone(),
	}
	digest, err := descriptorDigest(descriptor)
	if err != nil {
		return Descriptor{}, fmt.Errorf("%w: digest: %w", ErrInvalidDescriptor, err)
	}
	descriptor.digest = digest
	return descriptor, nil
}

// Name returns the stable Definition name.
func (d Descriptor) Name() string { return d.name }

// Description returns the human-readable purpose of the Definition.
func (d Descriptor) Description() string { return d.description }

// Version returns the canonical semantic version.
func (d Descriptor) Version() string { return d.version }

// InputSchema returns an independently owned schema value.
func (d Descriptor) InputSchema() Schema { return d.inputSchema.clone() }

// OutputSchema returns an independently owned schema value.
func (d Descriptor) OutputSchema() Schema { return d.outputSchema.clone() }

// Digest returns the SHA-256 identity of the complete descriptor contract.
func (d Descriptor) Digest() Digest { return d.digest }

// Valid reports whether the Descriptor was constructed successfully.
func (d Descriptor) Valid() bool {
	return d.name != "" && d.version != "" &&
		d.inputSchema.Valid() && d.outputSchema.Valid() && d.digest.Valid()
}

// ValidateInput validates an Input against the Definition contract.
func (d Descriptor) ValidateInput(input Input) error {
	if !d.Valid() {
		return ErrInvalidDescriptor
	}
	return d.inputSchema.ValidateInput(input)
}

// ValidateOutput validates an Output against the Definition contract.
func (d Descriptor) ValidateOutput(output Output) error {
	if !d.Valid() {
		return ErrInvalidDescriptor
	}
	return d.outputSchema.ValidateOutput(output)
}

// EncodeInput converts value into an Input and validates it against this
// Descriptor's authoritative input schema.
func (d Descriptor) EncodeInput[T any](value T) (Input, error) {
	if !d.Valid() {
		return Input{}, ErrInvalidDescriptor
	}
	input, err := EncodeInput(value)
	if err != nil {
		return Input{}, err
	}
	if err := d.ValidateInput(input); err != nil {
		return Input{}, err
	}
	return input, nil
}

// DecodeOutput validates output against this Descriptor's authoritative
// output schema and strictly decodes it into T.
func (d Descriptor) DecodeOutput[T any](output Output) (T, error) {
	var zero T
	if !d.Valid() {
		return zero, ErrInvalidDescriptor
	}
	if err := d.ValidateOutput(output); err != nil {
		return zero, err
	}
	return output.Decode[T]()
}

// MarshalJSON returns the validated static Definition contract.
func (d Descriptor) MarshalJSON() ([]byte, error) {
	if !d.Valid() {
		return nil, ErrInvalidDescriptor
	}
	return json.Marshal(descriptorWire{
		descriptorContractWire: d.contractWire(),
		Digest:                 d.digest,
	})
}

// UnmarshalJSON replaces d with a strictly decoded Descriptor.
func (d *Descriptor) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidDescriptor)
	}
	var wire descriptorWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidDescriptor, err)
	}
	if err := wireJSON.requireEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDescriptor, err)
	}
	inputSchema, err := ParseSchema(wire.InputSchema)
	if err != nil {
		return fmt.Errorf("%w: input schema: %w", ErrInvalidDescriptor, err)
	}
	outputSchema, err := ParseSchema(wire.OutputSchema)
	if err != nil {
		return fmt.Errorf("%w: output schema: %w", ErrInvalidDescriptor, err)
	}
	value, err := NewDescriptor(DescriptorConfig{
		Name:         wire.Name,
		Description:  wire.Description,
		Version:      wire.Version,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		return err
	}
	if wire.Digest != value.digest {
		return fmt.Errorf("%w: digest does not match descriptor content", ErrInvalidDescriptor)
	}
	*d = value
	return nil
}

type descriptorContractWire struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

type descriptorWire struct {
	descriptorContractWire
	Digest Digest `json:"digest"`
}

func validateDescriptorConfig(config DescriptorConfig) error {
	if !validQualifiedName(config.Name) {
		return fmt.Errorf("%w: name must start with a lowercase letter and contain only lowercase letters, digits, '.', '_' or '-'", ErrInvalidDescriptor)
	}
	if config.Description == "" || strings.TrimSpace(config.Description) != config.Description || len(config.Description) > maxDescriptionBytes {
		return fmt.Errorf("%w: description must be non-empty, trimmed, and at most %d bytes", ErrInvalidDescriptor, maxDescriptionBytes)
	}
	if !validSemanticVersion(config.Version) {
		return fmt.Errorf("%w: version must be a canonical MAJOR.MINOR.PATCH semantic version", ErrInvalidDescriptor)
	}
	if !config.InputSchema.Valid() {
		return fmt.Errorf("%w: input schema: %w", ErrInvalidDescriptor, ErrInvalidSchema)
	}
	if !config.OutputSchema.Valid() {
		return fmt.Errorf("%w: output schema: %w", ErrInvalidDescriptor, ErrInvalidSchema)
	}
	return nil
}

func validSemanticVersion(value string) bool {
	version, err := semver.StrictNewVersion(value)
	return err == nil && version.String() == value
}

func descriptorDigest(descriptor Descriptor) (Digest, error) {
	data, err := json.Marshal(descriptor.contractWire())
	if err != nil {
		return Digest{}, err
	}
	return digestBytes(data), nil
}

func (d Descriptor) contractWire() descriptorContractWire {
	return descriptorContractWire{
		Name:         d.name,
		Description:  d.description,
		Version:      d.version,
		InputSchema:  d.inputSchema.JSON(),
		OutputSchema: d.outputSchema.JSON(),
	}
}
