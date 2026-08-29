package platform

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	agent "github.com/Tangerg/scope/agent"
)

var (
	ErrInvalidCatalog = errors.New("platform: invalid deployment catalog")

	ErrDeploymentNotFound = errors.New("platform: deployment not found")
)

// Catalog is an immutable snapshot of exact Deployment bindings. It contains
// no active route, mutable registry, Process ownership, or remote discovery.
// The zero value is an empty catalog. Catalog values are safe for concurrent
// lookup and enumeration.
type Catalog struct {
	byReference map[agent.DeploymentRef]agent.Deployment
	ordered     []agent.Deployment
}

func NewCatalog(deployments ...agent.Deployment) (Catalog, error) {
	if len(deployments) == 0 {
		return Catalog{}, nil
	}
	byReference := make(map[agent.DeploymentRef]agent.Deployment, len(deployments))
	ordered := make([]agent.Deployment, len(deployments))
	for index, deployment := range deployments {
		if !deployment.Valid() {
			return Catalog{}, fmt.Errorf(
				"%w: deployment %d is invalid: %w",
				ErrInvalidCatalog, index, agent.ErrInvalidDeployment,
			)
		}
		reference := deployment.DeploymentRef()
		if _, duplicate := byReference[reference]; duplicate {
			return Catalog{}, fmt.Errorf(
				"%w: duplicate exact reference %s", ErrInvalidCatalog, reference,
			)
		}
		byReference[reference] = deployment
		ordered[index] = deployment
	}
	slices.SortFunc(ordered, compareDeployment)
	return Catalog{byReference: byReference, ordered: ordered}, nil
}

// Resolve returns the Deployment bound to one exact reference. It performs no
// routing or name fallback and satisfies agent.DeploymentResolver.
func (c Catalog) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	if !reference.Valid() {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	deployment, found := c.byReference[reference]
	if !found {
		return agent.Deployment{}, fmt.Errorf("%w: %s", ErrDeploymentNotFound, reference)
	}
	return deployment, nil
}

// Deployments returns all exact bindings in stable Definition-name and
// Deployment-digest order. The returned slice is independently owned and may
// be modified by the caller.
func (c Catalog) Deployments() []agent.Deployment {
	return slices.Clone(c.ordered)
}

func compareDeployment(left, right agent.Deployment) int {
	leftReference := left.DeploymentRef()
	rightReference := right.DeploymentRef()
	if order := cmp.Compare(leftReference.Name(), rightReference.Name()); order != 0 {
		return order
	}
	return cmp.Compare(leftReference.Digest().String(), rightReference.Digest().String())
}
