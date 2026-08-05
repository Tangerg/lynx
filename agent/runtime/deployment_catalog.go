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

// deploymentCatalog holds every immutable deployment an Engine has accepted and
// one active route per agent name. Historical definitions remain addressable
// by exact DeploymentRef until the host explicitly forgets them.
//
// Concurrency: a single RWMutex protects the map; deploys / undeploys
// are exclusive, lookups are shared. Used as a named field on Engine;
// methods are lowercase since the public API lives on Engine itself.
type deploymentCatalog struct {
	mu           sync.RWMutex
	active       map[string]core.DeploymentRef
	deployments  map[core.DeploymentRef]*Deployment
	retainCounts map[core.DeploymentRef]int
}

func newDeploymentCatalog() deploymentCatalog {
	return deploymentCatalog{
		active:       map[string]core.DeploymentRef{},
		deployments:  map[core.DeploymentRef]*Deployment{},
		retainCounts: map[core.DeploymentRef]int{},
	}
}

// activate installs candidate as the active route. Deploy uses replace=false:
// the same ref is idempotent and a different ref conflicts. Replace uses
// replace=true and requires an existing active route.
func (c *deploymentCatalog) activate(candidate *Deployment, replace bool) (*Deployment, bool, error) {
	if candidate == nil {
		return nil, false, errors.New("deployment catalog: candidate is nil")
	}
	ref := candidate.Ref()
	if err := ref.Validate(); err != nil {
		return nil, false, fmt.Errorf("deployment catalog: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	activeRef, active := c.active[ref.Name]
	if !active && replace {
		return nil, false, fmt.Errorf("%w: agent %q has no active deployment", ErrDeploymentNotFound, ref.Name)
	}
	if active && activeRef != ref && !replace {
		return nil, false, &DeploymentConflictError{Active: activeRef, Candidate: ref}
	}
	if active && activeRef == ref {
		existing := c.deployments[ref]
		return existing, false, nil
	}

	deployment := candidate
	if existing, ok := c.deployments[ref]; ok {
		deployment = existing
	} else {
		c.deployments[ref] = deployment
	}
	c.active[ref.Name] = ref
	return deployment, true, nil
}

// deactivate removes only the active route and retains the exact deployment.
func (c *deploymentCatalog) deactivate(name string) (*Deployment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ref, ok := c.active[name]
	if !ok {
		return nil, fmt.Errorf("%w: agent %q is not deployed", ErrDeploymentNotFound, name)
	}
	delete(c.active, name)
	return c.deployments[ref], nil
}

// listActive returns active deployments in stable agent-name order.
func (c *deploymentCatalog) listActive() []*Deployment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.active))
	for name := range c.active {
		names = append(names, name)
	}
	slices.Sort(names)
	deployments := make([]*Deployment, 0, len(names))
	for _, name := range names {
		deployments = append(deployments, c.deployments[c.active[name]])
	}
	return deployments
}

// listAll returns the historical catalog in stable ref order.
func (c *deploymentCatalog) listAll() []*Deployment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	refs := slices.Collect(maps.Keys(c.deployments))
	slices.SortFunc(refs, compareDeploymentRef)
	deployments := make([]*Deployment, len(refs))
	for i, ref := range refs {
		deployments[i] = c.deployments[ref]
	}
	return deployments
}

func (c *deploymentCatalog) activeDeployment(name string) (*Deployment, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key, ok := c.active[name]
	if !ok {
		return nil, false
	}
	deployment, ok := c.deployments[key]
	return deployment, ok
}

// lookup resolves an exact historical definition.
func (c *deploymentCatalog) lookup(ref core.DeploymentRef) (*Deployment, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	deployment, ok := c.deployments[ref]
	return deployment, ok
}

func (c *deploymentCatalog) retain(deployment *Deployment) bool {
	if deployment == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ref := deployment.Ref()
	if c.deployments[ref] != deployment {
		return false
	}
	c.retainCounts[ref]++
	return true
}

func (c *deploymentCatalog) release(deployment *Deployment) {
	if deployment == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ref := deployment.Ref()
	switch retainCount := c.retainCounts[ref]; {
	case retainCount > 1:
		c.retainCounts[ref] = retainCount - 1
	case retainCount == 1:
		delete(c.retainCounts, ref)
	}
}

func (c *deploymentCatalog) forget(ref core.DeploymentRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deployments[ref] == nil {
		return fmt.Errorf("%w: %s", ErrDeploymentNotFound, ref)
	}
	if c.active[ref.Name] == ref {
		return fmt.Errorf("%w: %s", ErrDeploymentActive, ref)
	}
	if c.retainCounts[ref] != 0 {
		return fmt.Errorf("%w: %s", ErrDeploymentInUse, ref)
	}
	delete(c.deployments, ref)
	delete(c.retainCounts, ref)
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
