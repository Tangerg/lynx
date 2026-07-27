package runtime

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// deploymentRegistry holds every immutable deployment an Engine has accepted and
// one active route per agent name. Historical definitions remain addressable
// by exact DeploymentRef until the host explicitly forgets them.
//
// Concurrency: a single RWMutex protects the map; deploys / undeploys
// are exclusive, lookups are shared. Used as a named field on Engine;
// methods are lowercase since the public API lives on Engine itself.
type deploymentRegistry struct {
	mu          sync.RWMutex
	active      map[string]core.DeploymentRef
	deployments map[core.DeploymentRef]*Deployment
	references  map[core.DeploymentRef]int
}

func newDeploymentRegistry() deploymentRegistry {
	return deploymentRegistry{
		active:      map[string]core.DeploymentRef{},
		deployments: map[core.DeploymentRef]*Deployment{},
		references:  map[core.DeploymentRef]int{},
	}
}

// activate installs candidate as the active route. Deploy uses replace=false:
// the same ref is idempotent and a different ref conflicts. Replace uses
// replace=true and requires an existing active route.
func (r *deploymentRegistry) activate(candidate *Deployment, replace bool) (*Deployment, bool, error) {
	if candidate == nil {
		return nil, false, errors.New("deployment catalog: candidate is nil")
	}
	ref := candidate.Ref()
	if err := ref.Validate(); err != nil {
		return nil, false, fmt.Errorf("deployment catalog: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	activeRef, active := r.active[ref.Name]
	if !active && replace {
		return nil, false, fmt.Errorf("%w: agent %q has no active deployment", ErrDeploymentNotFound, ref.Name)
	}
	if active && activeRef != ref && !replace {
		return nil, false, &DeploymentConflictError{Active: activeRef, Candidate: ref}
	}
	if active && activeRef == ref {
		existing := r.deployments[ref]
		return existing, false, nil
	}

	deployment := candidate
	if existing, ok := r.deployments[ref]; ok {
		deployment = existing
	} else {
		r.deployments[ref] = deployment
	}
	r.active[ref.Name] = ref
	return deployment, true, nil
}

// unregister removes only the active route and retains the exact deployment.
func (r *deploymentRegistry) unregister(name string) (*Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ref, ok := r.active[name]
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not deployed", ErrDeploymentNotFound, name)
	}
	delete(r.active, name)
	return r.deployments[ref], nil
}

// listActive returns active deployments in stable agent-name order.
func (r *deploymentRegistry) listActive() []*Deployment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.active))
	for name := range r.active {
		names = append(names, name)
	}
	slices.Sort(names)
	deployments := make([]*Deployment, 0, len(names))
	for _, name := range names {
		deployments = append(deployments, r.deployments[r.active[name]])
	}
	return deployments
}

// listAll returns the historical catalog in stable ref order.
func (r *deploymentRegistry) listAll() []*Deployment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	refs := slices.Collect(maps.Keys(r.deployments))
	slices.SortFunc(refs, compareDeploymentRef)
	deployments := make([]*Deployment, len(refs))
	for i, ref := range refs {
		deployments[i] = r.deployments[ref]
	}
	return deployments
}

func (r *deploymentRegistry) activeDeployment(name string) (*Deployment, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key, ok := r.active[name]
	if !ok {
		return nil, false
	}
	deployment, ok := r.deployments[key]
	return deployment, ok
}

// lookup resolves an exact historical definition.
func (r *deploymentRegistry) lookup(ref core.DeploymentRef) (*Deployment, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	deployment, ok := r.deployments[ref]
	return deployment, ok
}

func (r *deploymentRegistry) retain(deployment *Deployment) bool {
	if deployment == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ref := deployment.Ref()
	if r.deployments[ref] != deployment {
		return false
	}
	r.references[ref]++
	return true
}

func (r *deploymentRegistry) release(deployment *Deployment) {
	if deployment == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ref := deployment.Ref()
	switch references := r.references[ref]; {
	case references > 1:
		r.references[ref] = references - 1
	case references == 1:
		delete(r.references, ref)
	}
}

func (r *deploymentRegistry) forget(ref core.DeploymentRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deployments[ref] == nil {
		return fmt.Errorf("%w: %s", ErrDeploymentNotFound, ref)
	}
	if r.active[ref.Name] == ref {
		return fmt.Errorf("%w: %s", ErrDeploymentActive, ref)
	}
	if r.references[ref] != 0 {
		return fmt.Errorf("%w: %s", ErrDeploymentInUse, ref)
	}
	delete(r.deployments, ref)
	delete(r.references, ref)
	return nil
}

func compareDeploymentRef(a, b core.DeploymentRef) int {
	if n := cmp.Compare(a.Name, b.Name); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Version, b.Version); n != 0 {
		return n
	}
	return cmp.Compare(a.Digest, b.Digest)
}
