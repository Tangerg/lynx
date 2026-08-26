package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (p *processLoop) startChild(
	ctx context.Context,
	effectID EffectID,
	spec ChildSpec,
) ChildStartResult {
	if !spec.Valid() || !p.controller.relation.Valid() {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", ErrInvalidChildStart)
	}
	childID := deriveChildProcessID(effectID)
	relation := childProcessRelation(childID, p.controller.relation, spec.Key)
	requestDigest, err := childSpecDigest(spec)
	if err != nil {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", err)
	}
	if existing, exists := p.engine.Process(childID); exists {
		if existing.Relation() == relation && existing.DeploymentRef() == spec.DeploymentRef &&
			existing.controller.childRequestDigest == requestDigest {
			return ChildStartResult{key: spec.Key, processID: childID, deploymentRef: spec.DeploymentRef}
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.identity_conflict", ErrInvalidChildStart)
	}
	if !p.capabilities.Allows(spec.Capabilities) {
		return failedChildStart(spec, FailureKindContract, "engine.child.capability_escalation", ErrInvalidCapability)
	}
	if !p.reserveChildBudget(spec.Budget) {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_exhausted", ErrResourceLimitExceeded)
	}
	budgetCommitted := false
	defer func() {
		if !budgetCommitted {
			p.releaseChildBudget(spec.Budget)
		}
	}()
	childLimits, err := limitsFromBudget(p.limits, spec.Budget)
	if err != nil {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_invalid", err)
	}
	deployment, err := p.resolveChildDeployment(spec.DeploymentRef)
	if err != nil {
		return failedChildStart(spec, FailureKindExternal, "engine.child.deployment_unavailable", err)
	}
	if err := deployment.Descriptor().ValidateInput(spec.Input); err != nil {
		return failedChildStart(spec, FailureKindContract, "engine.child.input.invalid", err)
	}
	startedAt := time.Now().Round(0).UTC()
	admission := newProcessAdmission(
		relation, deployment, spec.Budget, spec.Capabilities, startedAt,
	)
	if err := p.engine.reserveProcessStart(
		relation, deployment.DeploymentRef(), p.treeLimits, requestDigest,
	); err != nil {
		if errors.Is(err, ErrResourceLimitExceeded) {
			return failedChildStart(spec, FailureKindExecution, "engine.child.tree_limit", err)
		}
		if errors.Is(err, ErrEngineClosed) {
			return failedChildStart(spec, FailureKindExternal, "engine.child.start.unavailable", err)
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.identity_conflict", err)
	}
	if err := requestProcessAdmission(ctx, p.engine.admitter, admission); err != nil {
		p.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, FailureKindExternal, "engine.child.admission.rejected", err,
		)
	}
	execution, state, failure, err := initializeExecution(deployment.Definition(), spec.Input)
	if err != nil {
		acknowledgeErr := p.engine.acknowledgeAbortedProcessOutcome(ctx, admission, failure)
		p.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, failure.Kind(), failure.Code(), errors.Join(err, acknowledgeErr),
		)
	}
	controller := newProcessController(
		relation, deployment.DeploymentRef(), spec.Budget, spec.Capabilities, p.treeLimits,
		startedAt, StatusRunning,
	)
	childLoop := newProcessLoop(
		p.engine, controller, deployment, execution, state, startedAt, childLimits,
	)
	if err := p.engine.acknowledgeStartedProcessOutcome(ctx, admission); err != nil {
		p.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, FailureKindExternal, "engine.child.start_outcome.unacknowledged", err,
		)
	}
	p.engine.publishReservedProcess(controller)
	budgetCommitted = true
	go childLoop.run(context.Background())
	return ChildStartResult{key: spec.Key, processID: childID, deploymentRef: spec.DeploymentRef}
}

func (p *processLoop) reserveChildBudget(requested Budget) bool {
	if !p.budget.canAllocate(p.usage, p.reservedBudget, requested) {
		return false
	}
	reserved, ok := p.reservedBudget.add(requested)
	if !ok {
		return false
	}
	p.reservedBudget = reserved
	return true
}

func (p *processLoop) releaseChildBudget(released Budget) {
	if released.Steps > p.reservedBudget.Steps ||
		released.Effects > p.reservedBudget.Effects ||
		released.Signals > p.reservedBudget.Signals {
		return
	}
	p.reservedBudget.Steps -= released.Steps
	p.reservedBudget.Effects -= released.Effects
	p.reservedBudget.Signals -= released.Signals
}

func (p *processLoop) resolveChildDeployment(
	reference DeploymentRef,
) (Deployment, error) {
	if reference == p.deployment.DeploymentRef() {
		return p.deployment, nil
	}
	if p.engine.resolver == nil {
		return Deployment{}, fmt.Errorf("%w: no resolver for %s", ErrInvalidDeployment, reference.Name())
	}
	deployment, err := resolveDeployment(p.engine.resolver, reference)
	if err != nil {
		return Deployment{}, err
	}
	if !deployment.Valid() || deployment.DeploymentRef() != reference {
		return Deployment{}, fmt.Errorf("%w: resolver returned a different binding", ErrInvalidDeployment)
	}
	return deployment, nil
}

func resolveDeployment(
	resolver DeploymentResolver,
	reference DeploymentRef,
) (deployment Deployment, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			deployment = Deployment{}
			err = fmt.Errorf("deployment resolver panicked: %v", recovered)
		}
	}()
	return resolver.Resolve(reference)
}

func failedChildStart(spec ChildSpec, kind FailureKind, code string, cause error) ChildStartResult {
	failure, err := failureFromError(kind, code, cause)
	if err != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.child.failure.invalid", "invalid child failure")
	}
	return ChildStartResult{key: spec.Key, deploymentRef: spec.DeploymentRef, failure: failure}
}

func childSpecDigest(spec ChildSpec) (Digest, error) {
	payload, err := json.Marshal(spec)
	if err != nil {
		return Digest{}, err
	}
	return digestBytes(payload), nil
}

func deriveChildProcessID(effectID EffectID) ProcessID {
	digest := digestBytes([]byte("child\x00" + effectID.String()))
	id, err := ParseProcessID("process:" + digest.String()[len("sha256:"):])
	if err != nil {
		panic(err)
	}
	return id
}
