package platform

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	agent "github.com/Tangerg/lynx/agent"
)

var (
	// ErrNilDeploymentSelector reports a nil selector implementation.
	ErrNilDeploymentSelector = errors.New("platform: nil deployment selector")

	// ErrNoDeploymentCandidates reports that the active snapshot is empty.
	ErrNoDeploymentCandidates = errors.New("platform: no deployment candidates")

	// ErrInvalidDeploymentSelection reports a selector panic or a reference
	// that was not present in the exact candidate snapshot it received.
	ErrInvalidDeploymentSelection = errors.New("platform: invalid deployment selection")
)

// DeploymentCandidate is one non-executable active binding offered to a
// DeploymentSelector. It exposes the exact identity and static Definition
// contract, never Dispatcher or Process lifecycle capabilities.
type DeploymentCandidate struct {
	reference  agent.DeploymentRef
	descriptor agent.Descriptor
}

// DeploymentRef returns the candidate's exact immutable Deployment identity.
func (d DeploymentCandidate) DeploymentRef() agent.DeploymentRef {
	return d.reference
}

// Descriptor returns the candidate's frozen static Definition contract.
func (d DeploymentCandidate) Descriptor() agent.Descriptor {
	return d.descriptor
}

// DeploymentSelector chooses one exact reference from a stable active
// candidate snapshot. Implementations may perform external I/O, must honor ctx,
// and must be safe for concurrent calls when shared. Request-specific routing
// input belongs to the implementation rather than a Framework payload type.
type DeploymentSelector interface {
	Select(ctx context.Context, candidates []DeploymentCandidate) (agent.DeploymentRef, error)
}

// DeploymentSelectorFunc adapts a function to DeploymentSelector.
type DeploymentSelectorFunc func(
	ctx context.Context,
	candidates []DeploymentCandidate,
) (agent.DeploymentRef, error)

// Select invokes d.
func (d DeploymentSelectorFunc) Select(
	ctx context.Context,
	candidates []DeploymentCandidate,
) (agent.DeploymentRef, error) {
	return d(ctx, candidates)
}

// DeploymentCandidates returns a stable snapshot of active, non-executable
// candidates. Replaced, undeployed, and other historical Catalog bindings are
// intentionally excluded. The returned slice is independently owned.
func (p *Platform) DeploymentCandidates() []DeploymentCandidate {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return candidatesFrom(p.state.ordered)
}

// SelectDeployment asks selector to choose from one stable active snapshot and
// returns the exact Deployment captured in that snapshot. Concurrent Replace or
// Undeploy cannot redirect the completed selection to a different binding.
func (p *Platform) SelectDeployment(
	ctx context.Context,
	selector DeploymentSelector,
) (agent.Deployment, error) {
	if p == nil {
		return agent.Deployment{}, ErrNilPlatform
	}
	if nilDeploymentSelector(selector) {
		return agent.Deployment{}, ErrNilDeploymentSelector
	}
	p.mu.RLock()
	deployments := slices.Clone(p.state.ordered)
	p.mu.RUnlock()
	if len(deployments) == 0 {
		return agent.Deployment{}, ErrNoDeploymentCandidates
	}
	candidates := candidatesFrom(deployments)
	offered := make(map[agent.DeploymentRef]agent.Deployment, len(deployments))
	for _, deployment := range deployments {
		offered[deployment.DeploymentRef()] = deployment
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reference, err := callDeploymentSelector(ctx, selector, slices.Clone(candidates))
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("platform: select Deployment: %w", err)
	}
	if !reference.Valid() {
		return agent.Deployment{}, fmt.Errorf(
			"%w: selector returned an invalid exact reference",
			ErrInvalidDeploymentSelection,
		)
	}
	deployment, found := offered[reference]
	if !found {
		return agent.Deployment{}, fmt.Errorf(
			"%w: selector returned unoffered reference %s",
			ErrInvalidDeploymentSelection, reference,
		)
	}
	return deployment, nil
}

func candidatesFrom(deployments []agent.Deployment) []DeploymentCandidate {
	candidates := make([]DeploymentCandidate, len(deployments))
	for index, deployment := range deployments {
		candidates[index] = DeploymentCandidate{
			reference: deployment.DeploymentRef(), descriptor: deployment.Descriptor(),
		}
	}
	return candidates
}

func callDeploymentSelector(
	ctx context.Context,
	selector DeploymentSelector,
	candidates []DeploymentCandidate,
) (reference agent.DeploymentRef, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			reference = agent.DeploymentRef{}
			err = fmt.Errorf("%w: selector panicked: %v", ErrInvalidDeploymentSelection, recovered)
		}
	}()
	return selector.Select(ctx, candidates)
}

func nilDeploymentSelector(selector DeploymentSelector) bool {
	if selector == nil {
		return true
	}
	value := reflect.ValueOf(selector)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
