package agent

import (
	"errors"
	"fmt"

	"github.com/samber/lo"
)

var ErrInvalidDeployment = errors.New("agent: invalid deployment")

// DeploymentConfig contains the complete behavior binding of one Deployment.
// The digests must cover the exact code artifact and all frozen dispatcher or
// Strategy configuration that can affect execution or restoration.
type DeploymentConfig struct {
	// Definition owns the Strategy contract and creates per-Process execution.
	Definition Definition

	// Dispatcher interprets only Effects emitted by this Definition.
	Dispatcher Dispatcher

	// ImplementationDigest identifies the exact executable Definition artifact.
	ImplementationDigest Digest

	// ConfigurationDigest identifies all frozen behavior-affecting Definition
	// and Dispatcher configuration.
	ConfigurationDigest Digest
}

// Deployment is an immutable binding of one Definition, its Strategy-owned
// Dispatcher, and an exact value reference used by Process snapshots.
type Deployment struct {
	reference  DeploymentRef
	descriptor Descriptor
	definition Definition
	dispatcher Dispatcher
}

func NewDeployment(config DeploymentConfig) (Deployment, error) {
	if lo.IsNil(config.Definition) {
		return Deployment{}, fmt.Errorf("%w: definition is required", ErrInvalidDeployment)
	}
	if lo.IsNil(config.Dispatcher) {
		return Deployment{}, fmt.Errorf("%w: dispatcher is required", ErrInvalidDeployment)
	}
	descriptor := config.Definition.Descriptor()
	reference, err := NewDeploymentRef(descriptor, config.ImplementationDigest, config.ConfigurationDigest)
	if err != nil {
		return Deployment{}, fmt.Errorf("%w: %w", ErrInvalidDeployment, err)
	}
	return Deployment{
		reference:  reference,
		descriptor: descriptor,
		definition: config.Definition,
		dispatcher: config.Dispatcher,
	}, nil
}

// DeploymentRef returns the exact value identity stored in Process snapshots.
func (d Deployment) DeploymentRef() DeploymentRef { return d.reference }

// Descriptor returns the frozen static Definition contract.
func (d Deployment) Descriptor() Descriptor { return d.descriptor }

// Definition returns the erased behavior definition bound to this Deployment.
func (d Deployment) Definition() Definition { return d.definition }

func (d Deployment) Valid() bool {
	return d.reference.Valid() && d.descriptor.Valid() &&
		!lo.IsNil(d.definition) && !lo.IsNil(d.dispatcher) &&
		d.definition.Descriptor().Digest() == d.descriptor.Digest()
}

func (d Deployment) effectDispatcher() Dispatcher { return d.dispatcher }
