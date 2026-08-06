package platform

import (
	"errors"
	"fmt"
	"maps"

	agent "github.com/Tangerg/lynx/agent2"
)

var (
	// ErrNilPlatform reports a nil Platform receiver.
	ErrNilPlatform = errors.New("platform: nil platform")

	// ErrDeploymentConflict reports that a name/version slot is bound to a
	// different exact Deployment.
	ErrDeploymentConflict = errors.New("platform: deployment conflict")

	// ErrDeploymentNotActive reports that a name/version slot has no active
	// Deployment to replace or undeploy.
	ErrDeploymentNotActive = errors.New("platform: deployment is not active")
)

// DeploymentConflictError identifies the active and requested exact bindings
// that collided in one Definition-name and semantic-version slot.
type DeploymentConflictError struct {
	// Active is the exact binding currently occupying the version slot.
	Active agent.DeploymentRef

	// Requested is the exact binding the caller attempted to deploy or undeploy.
	Requested agent.DeploymentRef
}

// Error describes both exact bindings in the conflicting version slot.
func (conflict *DeploymentConflictError) Error() string {
	if conflict == nil {
		return ErrDeploymentConflict.Error()
	}
	return fmt.Sprintf(
		"%s: active %s, requested %s",
		ErrDeploymentConflict, conflict.Active, conflict.Requested,
	)
}

// Unwrap supports errors.Is with ErrDeploymentConflict.
func (*DeploymentConflictError) Unwrap() error { return ErrDeploymentConflict }

func newDeploymentConflict(active, requested agent.DeploymentRef) error {
	return &DeploymentConflictError{Active: active, Requested: requested}
}

// Deploy activates deployment in its Definition-name and semantic-version
// slot. Reapplying the exact active binding leaves the Platform unchanged; a
// different exact binding in the same slot returns ErrDeploymentConflict and
// requires Replace.
func (platform *Platform) Deploy(deployment agent.Deployment) error {
	if platform == nil {
		return ErrNilPlatform
	}
	platform.mu.Lock()
	defer platform.mu.Unlock()
	if !deployment.Valid() {
		return agent.ErrInvalidDeployment
	}
	reference := deployment.DeploymentRef()
	slot := slotFor(reference)
	if current, occupied := platform.state.active[slot]; occupied {
		if current == reference {
			return nil
		}
		return newDeploymentConflict(current, reference)
	}
	catalog, err := catalogWith(platform.state.catalog, deployment)
	if err != nil {
		return fmt.Errorf("platform: deploy %s: %w", reference, err)
	}
	active := maps.Clone(platform.state.active)
	if active == nil {
		active = make(map[deploymentSlot]agent.DeploymentRef)
	}
	active[slot] = reference
	state, err := deploymentStateFrom(catalog, active)
	if err != nil {
		return fmt.Errorf("platform: deploy %s: %w", reference, err)
	}
	platform.state = state
	return nil
}

// Replace changes the active exact binding in deployment's existing name and
// version slot. The previous binding remains in Catalog for exact restoration.
// A new semantic version must be introduced with Deploy, not Replace.
func (platform *Platform) Replace(deployment agent.Deployment) error {
	if platform == nil {
		return ErrNilPlatform
	}
	platform.mu.Lock()
	defer platform.mu.Unlock()
	if !deployment.Valid() {
		return agent.ErrInvalidDeployment
	}
	reference := deployment.DeploymentRef()
	slot := slotFor(reference)
	current, occupied := platform.state.active[slot]
	if !occupied {
		return fmt.Errorf("%w: %s", ErrDeploymentNotActive, slot)
	}
	if current == reference {
		return nil
	}
	catalog, err := catalogWith(platform.state.catalog, deployment)
	if err != nil {
		return fmt.Errorf("platform: replace %s: %w", reference, err)
	}
	active := maps.Clone(platform.state.active)
	active[slot] = reference
	state, err := deploymentStateFrom(catalog, active)
	if err != nil {
		return fmt.Errorf("platform: replace %s: %w", reference, err)
	}
	platform.state = state
	return nil
}

// Undeploy removes reference only when it is the exact active binding in its
// name/version slot. A stale reference returns ErrDeploymentConflict instead
// of deactivating a replacement. The exact binding remains in Catalog.
func (platform *Platform) Undeploy(reference agent.DeploymentRef) error {
	if platform == nil {
		return ErrNilPlatform
	}
	platform.mu.Lock()
	defer platform.mu.Unlock()
	if !reference.Valid() {
		return agent.ErrInvalidDeploymentRef
	}
	slot := slotFor(reference)
	current, occupied := platform.state.active[slot]
	if !occupied {
		return fmt.Errorf("%w: %s", ErrDeploymentNotActive, slot)
	}
	if current != reference {
		return newDeploymentConflict(current, reference)
	}
	active := maps.Clone(platform.state.active)
	delete(active, slot)
	state, err := deploymentStateFrom(platform.state.catalog, active)
	if err != nil {
		return fmt.Errorf("platform: undeploy %s: %w", reference, err)
	}
	platform.state = state
	return nil
}

func catalogWith(catalog Catalog, deployment agent.Deployment) (Catalog, error) {
	if _, err := catalog.Resolve(deployment.DeploymentRef()); err == nil {
		return catalog, nil
	} else if !errors.Is(err, ErrDeploymentNotFound) {
		return Catalog{}, err
	}
	deployments := catalog.Deployments()
	deployments = append(deployments, deployment)
	return NewCatalog(deployments...)
}
