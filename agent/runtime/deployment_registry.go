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
// by exact DeploymentRef after replacement or undeploy.
//
// Concurrency: a single RWMutex protects the map; deploys / undeploys
// are exclusive, lookups are shared. Used as a named field on Engine;
// methods are lowercase since the public API lives on Engine itself.
type deploymentRegistry struct {
	mu          sync.RWMutex
	active      map[string]core.DeploymentRef
	deployments map[core.DeploymentRef]*Deployment
	sources     map[*core.Agent]core.DeploymentRef
}

func newDeploymentRegistry() deploymentRegistry {
	return deploymentRegistry{
		active:      map[string]core.DeploymentRef{},
		deployments: map[core.DeploymentRef]*Deployment{},
		sources:     map[*core.Agent]core.DeploymentRef{},
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
		r.sources[candidate.source] = ref
		return existing, false, nil
	}

	deployment := candidate
	if existing, ok := r.deployments[ref]; ok {
		deployment = existing
	} else {
		r.deployments[ref] = deployment
	}
	r.active[ref.Name] = ref
	r.sources[candidate.source] = ref
	r.sources[deployment.agent] = ref
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

func (r *deploymentRegistry) forSource(source *core.Agent) (*Deployment, bool) {
	if source == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	key, ok := r.sources[source]
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

func compareDeploymentRef(a, b core.DeploymentRef) int {
	if n := cmp.Compare(a.Name, b.Name); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Version, b.Version); n != 0 {
		return n
	}
	return cmp.Compare(a.Digest, b.Digest)
}
