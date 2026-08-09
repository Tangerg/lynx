package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidDeploymentRef reports a malformed exact Deployment identity.
var ErrInvalidDeploymentRef = errors.New("agent: invalid deployment reference")

const invalidDeploymentRefText = "<invalid-deployment-ref>"

// DeploymentRef is the immutable value identity of one exact Definition
// implementation and frozen execution configuration. It contains no registry
// pointer and is sufficient to reject restore against a different Deployment.
type DeploymentRef struct {
	name                 string
	version              string
	contractDigest       Digest
	implementationDigest Digest
	configurationDigest  Digest
	digest               Digest
}

// NewDeploymentRef binds a validated Descriptor contract to exact code and
// frozen configuration identities. The caller that assembles a Deployment is
// responsible for hashing all behavior-affecting implementation and dispatcher
// configuration into the two supplied digests.
func NewDeploymentRef(descriptor Descriptor, implementationDigest, configurationDigest Digest) (DeploymentRef, error) {
	if !descriptor.Valid() {
		return DeploymentRef{}, fmt.Errorf("%w: %w", ErrInvalidDeploymentRef, ErrInvalidDescriptor)
	}
	if !implementationDigest.Valid() {
		return DeploymentRef{}, fmt.Errorf("%w: implementation: %w", ErrInvalidDeploymentRef, ErrInvalidDigest)
	}
	if !configurationDigest.Valid() {
		return DeploymentRef{}, fmt.Errorf("%w: configuration: %w", ErrInvalidDeploymentRef, ErrInvalidDigest)
	}
	reference := DeploymentRef{
		name:                 descriptor.Name(),
		version:              descriptor.Version(),
		contractDigest:       descriptor.Digest(),
		implementationDigest: implementationDigest,
		configurationDigest:  configurationDigest,
	}
	digest, err := deploymentDigest(reference)
	if err != nil {
		return DeploymentRef{}, fmt.Errorf("%w: digest: %w", ErrInvalidDeploymentRef, err)
	}
	reference.digest = digest
	return reference, nil
}

// Name returns the stable Definition name.
func (reference DeploymentRef) Name() string { return reference.name }

// Version returns the canonical semantic Definition version.
func (reference DeploymentRef) Version() string { return reference.version }

// ContractDigest returns the exact Descriptor contract identity.
func (reference DeploymentRef) ContractDigest() Digest { return reference.contractDigest }

// ImplementationDigest returns the exact executable implementation identity.
func (reference DeploymentRef) ImplementationDigest() Digest { return reference.implementationDigest }

// ConfigurationDigest returns the frozen behavior-affecting configuration
// identity, including dispatcher configuration.
func (reference DeploymentRef) ConfigurationDigest() Digest { return reference.configurationDigest }

// Digest returns the complete Deployment value identity.
func (reference DeploymentRef) Digest() Digest { return reference.digest }

// String returns a compact diagnostic form containing the stable name,
// semantic version, and complete Deployment digest. It is not a wire encoding.
func (reference DeploymentRef) String() string {
	if !reference.Valid() {
		return invalidDeploymentRefText
	}
	return reference.name + "@" + reference.version + "+" + reference.digest.String()
}

// Valid reports whether all identity components and their derived digest agree.
func (reference DeploymentRef) Valid() bool {
	if !validQualifiedName(reference.name) || !validSemanticVersion(reference.version) ||
		!reference.contractDigest.Valid() || !reference.implementationDigest.Valid() ||
		!reference.configurationDigest.Valid() || !reference.digest.Valid() {
		return false
	}
	want, err := deploymentDigest(reference)
	return err == nil && want == reference.digest
}

// MarshalJSON returns the validated exact Deployment identity.
func (reference DeploymentRef) MarshalJSON() ([]byte, error) {
	if !reference.Valid() {
		return nil, ErrInvalidDeploymentRef
	}
	return json.Marshal(deploymentRefWire{
		deploymentIdentityWire: reference.identityWire(),
		Digest:                 reference.digest,
	})
}

// UnmarshalJSON replaces reference with a strictly decoded exact identity.
func (reference *DeploymentRef) UnmarshalJSON(data []byte) error {
	if reference == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidDeploymentRef)
	}
	var wire deploymentRefWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidDeploymentRef, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDeploymentRef, err)
	}
	value := DeploymentRef{
		name:                 wire.Name,
		version:              wire.Version,
		contractDigest:       wire.ContractDigest,
		implementationDigest: wire.ImplementationDigest,
		configurationDigest:  wire.ConfigurationDigest,
		digest:               wire.Digest,
	}
	if !value.Valid() {
		return fmt.Errorf("%w: digest or identity component does not match", ErrInvalidDeploymentRef)
	}
	*reference = value
	return nil
}

type deploymentIdentityWire struct {
	Name                 string `json:"name"`
	Version              string `json:"version"`
	ContractDigest       Digest `json:"contract_digest"`
	ImplementationDigest Digest `json:"implementation_digest"`
	ConfigurationDigest  Digest `json:"configuration_digest"`
}

type deploymentRefWire struct {
	deploymentIdentityWire
	Digest Digest `json:"digest"`
}

func (reference DeploymentRef) identityWire() deploymentIdentityWire {
	return deploymentIdentityWire{
		Name:                 reference.name,
		Version:              reference.version,
		ContractDigest:       reference.contractDigest,
		ImplementationDigest: reference.implementationDigest,
		ConfigurationDigest:  reference.configurationDigest,
	}
}

func deploymentDigest(reference DeploymentRef) (Digest, error) {
	data, err := json.Marshal(reference.identityWire())
	if err != nil {
		return Digest{}, err
	}
	return digestBytes(data), nil
}
