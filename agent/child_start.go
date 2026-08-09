package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (loop *processLoop) startChild(
	ctx context.Context,
	effectID EffectID,
	spec ChildSpec,
) ChildStartResult {
	if !spec.Valid() || !loop.controller.relation.Valid() {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", ErrInvalidChildStart)
	}
	childID := deriveChildProcessID(effectID)
	relation := childProcessRelation(childID, loop.controller.relation, spec.Key)
	requestDigest, err := childSpecDigest(spec)
	if err != nil {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", err)
	}
	if existing, exists := loop.engine.Process(childID); exists {
		if existing.Relation() == relation && existing.DeploymentRef() == spec.DeploymentRef &&
			existing.controller.childRequestDigest == requestDigest {
			return ChildStartResult{key: spec.Key, processID: childID, deploymentRef: spec.DeploymentRef}
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.identity_conflict", ErrInvalidChildStart)
	}
	if !loop.capabilities.Allows(spec.Capabilities) {
		return failedChildStart(spec, FailureKindContract, "engine.child.capability_escalation", ErrInvalidCapability)
	}
	if !loop.reserveChildBudget(spec.Budget) {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_exhausted", ErrResourceLimitExceeded)
	}
	budgetCommitted := false
	defer func() {
		if !budgetCommitted {
			loop.releaseChildBudget(spec.Budget)
		}
	}()
	childLimits, err := limitsFromBudget(loop.limits, spec.Budget)
	if err != nil {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_invalid", err)
	}
	deployment, err := loop.resolveChildDeployment(spec.DeploymentRef)
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
	if err := loop.engine.reserveProcessStart(
		relation, deployment.DeploymentRef(), loop.treeLimits, requestDigest,
	); err != nil {
		if errors.Is(err, ErrResourceLimitExceeded) {
			return failedChildStart(spec, FailureKindExecution, "engine.child.tree_limit", err)
		}
		if errors.Is(err, ErrEngineClosed) {
			return failedChildStart(spec, FailureKindExternal, "engine.child.start.unavailable", err)
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.identity_conflict", err)
	}
	if err := requestProcessAdmission(ctx, loop.engine.admitter, admission); err != nil {
		loop.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, FailureKindExternal, "engine.child.admission.rejected", err,
		)
	}
	execution, state, failure, err := initializeExecution(deployment.Definition(), spec.Input)
	if err != nil {
		acknowledgeErr := loop.engine.acknowledgeAbortedProcessOutcome(ctx, admission, failure)
		loop.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, failure.Kind(), failure.Code(), errors.Join(err, acknowledgeErr),
		)
	}
	controller := newProcessController(
		relation, deployment.DeploymentRef(), spec.Budget, spec.Capabilities, loop.treeLimits,
		startedAt, StatusRunning,
	)
	childLoop := newProcessLoop(
		loop.engine, controller, deployment, execution, state, startedAt, childLimits,
	)
	if err := loop.engine.acknowledgeStartedProcessOutcome(ctx, admission); err != nil {
		loop.engine.discardProcessStartReservation(childID)
		return failedChildStart(
			spec, FailureKindExternal, "engine.child.start_outcome.unacknowledged", err,
		)
	}
	loop.engine.publishReservedProcess(controller)
	budgetCommitted = true
	go childLoop.run(context.Background())
	return ChildStartResult{key: spec.Key, processID: childID, deploymentRef: spec.DeploymentRef}
}

func (loop *processLoop) reserveChildBudget(requested Budget) bool {
	if !loop.budget.canAllocate(loop.usage, loop.reservedBudget, requested) {
		return false
	}
	reserved, ok := loop.reservedBudget.add(requested)
	if !ok {
		return false
	}
	loop.reservedBudget = reserved
	return true
}

func (loop *processLoop) releaseChildBudget(released Budget) {
	if released.Steps > loop.reservedBudget.Steps ||
		released.Effects > loop.reservedBudget.Effects ||
		released.Signals > loop.reservedBudget.Signals {
		return
	}
	loop.reservedBudget.Steps -= released.Steps
	loop.reservedBudget.Effects -= released.Effects
	loop.reservedBudget.Signals -= released.Signals
}

func (loop *processLoop) resolveChildDeployment(
	reference DeploymentRef,
) (Deployment, error) {
	if reference == loop.deployment.DeploymentRef() {
		return loop.deployment, nil
	}
	if loop.engine.resolver == nil {
		return Deployment{}, fmt.Errorf("%w: no resolver for %s", ErrInvalidDeployment, reference.Name())
	}
	deployment, err := resolveDeployment(loop.engine.resolver, reference)
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
