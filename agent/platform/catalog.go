package platform

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Masterminds/semver/v3"

	agent "github.com/Tangerg/lynx/agent"
)

var (
	// ErrInvalidCatalog reports an invalid or duplicate Deployment binding.
	ErrInvalidCatalog = errors.New("platform: invalid deployment catalog")

	// ErrDeploymentNotFound reports that an exact DeploymentRef is absent.
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

// NewCatalog validates and snapshots deployments. Multiple Deployments may
// share a Definition name or semantic version when their exact references
// differ; a duplicate exact DeploymentRef is rejected instead of overwritten.
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
// routing or version fallback and satisfies agent.DeploymentResolver.
func (catalog Catalog) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	if !reference.Valid() {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	deployment, found := catalog.byReference[reference]
	if !found {
		return agent.Deployment{}, fmt.Errorf("%w: %s", ErrDeploymentNotFound, reference)
	}
	return deployment, nil
}

// Deployments returns all exact bindings in stable Definition-name, semantic-
// version, and Deployment-digest order. The returned slice is independently
// owned and may be modified by the caller.
func (catalog Catalog) Deployments() []agent.Deployment {
	return slices.Clone(catalog.ordered)
}

func compareDeployment(left, right agent.Deployment) int {
	leftReference := left.DeploymentRef()
	rightReference := right.DeploymentRef()
	if order := cmp.Compare(leftReference.Name(), rightReference.Name()); order != 0 {
		return order
	}
	leftVersion, _ := semver.StrictNewVersion(leftReference.Version())
	rightVersion, _ := semver.StrictNewVersion(rightReference.Version())
	if order := leftVersion.Compare(rightVersion); order != 0 {
		return order
	}
	return cmp.Compare(leftReference.Digest().String(), rightReference.Digest().String())
}
