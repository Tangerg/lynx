package platform

import (
	"fmt"
	"slices"
	"sync"

	agent "github.com/Tangerg/lynx/agent2"
)

// Config contains the complete initial Platform deployment state. Each entry
// is initially active in its Definition-name and semantic-version slot.
type Config struct {
	// Deployments are the exact bindings initially active in the Platform.
	Deployments []agent.Deployment
}

// Platform owns atomic deployment changes, active version slots, and an exact
// historical Catalog above Engine. It does not own Process lifecycle or Host
// persistence. A Platform must be constructed with New and must not be copied.
type Platform struct {
	mu          sync.RWMutex
	initialized bool
	state       deploymentState
}

type deploymentState struct {
	catalog Catalog
	active  map[deploymentSlot]agent.DeploymentRef
	ordered []agent.Deployment
}

type deploymentSlot struct {
	name    string
	version string
}

func (slot deploymentSlot) String() string { return slot.name + "@" + slot.version }

func slotFor(reference agent.DeploymentRef) deploymentSlot {
	return deploymentSlot{name: reference.Name(), version: reference.Version()}
}

// New validates and atomically constructs the initial Platform. Deployments
// with different semantic versions occupy independent active slots; different
// exact references for the same name and version conflict.
func New(config Config) (*Platform, error) {
	state, err := initialDeploymentState(config.Deployments)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return &Platform{initialized: true, state: state}, nil
}

// Catalog returns the current immutable exact binding snapshot. Historical
// Deployments remain present after Replace and Undeploy.
func (platform *Platform) Catalog() Catalog {
	if platform == nil {
		return Catalog{}
	}
	platform.mu.RLock()
	defer platform.mu.RUnlock()
	if !platform.initialized {
		return Catalog{}
	}
	return platform.state.catalog
}

// ActiveDeployments returns the current active bindings in stable Definition-
// name, semantic-version, and Deployment-digest order.
func (platform *Platform) ActiveDeployments() []agent.Deployment {
	if platform == nil {
		return nil
	}
	platform.mu.RLock()
	defer platform.mu.RUnlock()
	if !platform.initialized {
		return nil
	}
	return slices.Clone(platform.state.ordered)
}

// Resolve performs one exact lookup against the current immutable Catalog and
// satisfies agent2.DeploymentResolver. Inactive historical bindings remain
// resolvable so Process restoration never follows an active route by mistake.
func (platform *Platform) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	if platform == nil {
		return agent.Deployment{}, ErrInvalidPlatform
	}
	platform.mu.RLock()
	if !platform.initialized {
		platform.mu.RUnlock()
		return agent.Deployment{}, ErrInvalidPlatform
	}
	catalog := platform.state.catalog
	platform.mu.RUnlock()
	return catalog.Resolve(reference)
}

func initialDeploymentState(deployments []agent.Deployment) (deploymentState, error) {
	catalog, err := NewCatalog(deployments...)
	if err != nil {
		return deploymentState{}, err
	}
	active := make(map[deploymentSlot]agent.DeploymentRef, len(deployments))
	for _, deployment := range deployments {
		reference := deployment.Reference()
		slot := slotFor(reference)
		if current, occupied := active[slot]; occupied && current != reference {
			return deploymentState{}, newDeploymentConflict(current, reference)
		}
		active[slot] = reference
	}
	return deploymentStateFrom(catalog, active)
}

func deploymentStateFrom(
	catalog Catalog,
	active map[deploymentSlot]agent.DeploymentRef,
) (deploymentState, error) {
	ordered := make([]agent.Deployment, 0, len(active))
	for _, reference := range active {
		deployment, err := catalog.Resolve(reference)
		if err != nil {
			return deploymentState{}, fmt.Errorf("platform: active deployment %s: %w", reference, err)
		}
		ordered = append(ordered, deployment)
	}
	slices.SortFunc(ordered, compareDeployment)
	return deploymentState{catalog: catalog, active: active, ordered: ordered}, nil
}
