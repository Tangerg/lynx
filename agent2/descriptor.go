package agent2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	maxDefinitionNameBytes = 128
	maxDescriptionBytes    = 4096
)

var ErrInvalidDescriptor = errors.New("agent: invalid descriptor")

// DescriptorConfig contains the complete static contract of a Definition.
// Executable implementation and frozen configuration identity belong to a
// Deployment, not this contract.
type DescriptorConfig struct {
	Name         string
	Description  string
	Version      string
	InputSchema  Schema
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
	digest       string
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

// Digest returns the sha256 identity of the complete descriptor contract.
func (d Descriptor) Digest() string { return d.digest }

// Valid reports whether the Descriptor was constructed successfully.
func (d Descriptor) Valid() bool {
	return d.name != "" && d.version != "" &&
		d.inputSchema.Valid() && d.outputSchema.Valid() && validDigest(d.digest)
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

func (d Descriptor) MarshalJSON() ([]byte, error) {
	if !d.Valid() {
		return nil, ErrInvalidDescriptor
	}
	return json.Marshal(descriptorWire{
		Name:         d.name,
		Description:  d.description,
		Version:      d.version,
		InputSchema:  d.inputSchema.JSON(),
		OutputSchema: d.outputSchema.JSON(),
		Digest:       d.digest,
	})
}

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
	if err := requireJSONEOF(decoder); err != nil {
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

type descriptorWire struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Version      string          `json:"version"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Digest       string          `json:"digest,omitempty"`
}

func validateDescriptorConfig(config DescriptorConfig) error {
	if !validDefinitionName(config.Name) {
		return fmt.Errorf("%w: name must start with a lowercase letter and contain only lowercase letters, digits, '.', '_' or '-'", ErrInvalidDescriptor)
	}
	if config.Description == "" || strings.TrimSpace(config.Description) != config.Description || len(config.Description) > maxDescriptionBytes {
		return fmt.Errorf("%w: description must be non-empty, trimmed, and at most %d bytes", ErrInvalidDescriptor, maxDescriptionBytes)
	}
	version, err := semver.StrictNewVersion(config.Version)
	if err != nil || version.String() != config.Version {
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

func validDefinitionName(name string) bool {
	if len(name) == 0 || len(name) > maxDefinitionNameBytes || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func descriptorDigest(descriptor Descriptor) (string, error) {
	data, err := json.Marshal(descriptorWire{
		Name:         descriptor.name,
		Description:  descriptor.description,
		Version:      descriptor.version,
		InputSchema:  descriptor.inputSchema.JSON(),
		OutputSchema: descriptor.outputSchema.JSON(),
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validDigest(digest string) bool {
	encoded, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(encoded) != sha256.Size*2 || encoded != strings.ToLower(encoded) {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}
