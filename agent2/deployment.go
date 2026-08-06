package agent2

import (
	"errors"
	"fmt"
	"reflect"
)

// ErrInvalidDeployment reports an incomplete or contradictory exact binding.
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

// NewDeployment validates and freezes the identity of a complete execution
// binding. Definition and Dispatcher implementations must themselves obey their
// documented immutability and concurrency contracts.
func NewDeployment(config DeploymentConfig) (Deployment, error) {
	if nilInterface(config.Definition) {
		return Deployment{}, fmt.Errorf("%w: definition is required", ErrInvalidDeployment)
	}
	if nilInterface(config.Dispatcher) {
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
func (deployment Deployment) DeploymentRef() DeploymentRef { return deployment.reference }

// Descriptor returns the frozen static Definition contract.
func (deployment Deployment) Descriptor() Descriptor { return deployment.descriptor }

// Definition returns the erased behavior definition bound to this Deployment.
func (deployment Deployment) Definition() Definition { return deployment.definition }

// Valid reports whether the binding remains complete and its Definition still
// reports the contract frozen at construction.
func (deployment Deployment) Valid() bool {
	return deployment.reference.Valid() && deployment.descriptor.Valid() &&
		!nilInterface(deployment.definition) && !nilInterface(deployment.dispatcher) &&
		deployment.definition.Descriptor().Digest() == deployment.descriptor.Digest()
}

func (deployment Deployment) effectDispatcher() Dispatcher { return deployment.dispatcher }

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
