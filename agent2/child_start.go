package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (runtime *processRuntime) startChild(
	ctx context.Context,
	effectID EffectID,
	spec ChildSpec,
) ChildStartResult {
	if !spec.Valid() || !runtime.controller.relation.Valid() {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", ErrInvalidChild)
	}
	childID := deriveChildProcessID(effectID)
	relation := childProcessRelation(childID, runtime.controller.relation, spec.Key)
	requestDigest, err := childSpecDigest(spec)
	if err != nil {
		return failedChildStart(spec, FailureKindContract, "engine.child.request.invalid", err)
	}
	if existing, exists := runtime.engine.Process(childID); exists {
		if existing.Relation() == relation && existing.DeploymentRef() == spec.Deployment &&
			existing.controller.childRequestDigest == requestDigest {
			return ChildStartResult{key: spec.Key, processID: childID, deployment: spec.Deployment}
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.identity_conflict", ErrInvalidChild)
	}
	if !runtime.capabilities.Allows(spec.Capabilities) {
		return failedChildStart(spec, FailureKindContract, "engine.child.capability_escalation", ErrInvalidCapability)
	}
	if !runtime.reserveChildBudget(spec.Budget) {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_exhausted", ErrLimitExceeded)
	}
	budgetCommitted := false
	defer func() {
		if !budgetCommitted {
			runtime.releaseChildBudget(spec.Budget)
		}
	}()
	childLimits, err := limitsFromBudget(runtime.limits, spec.Budget)
	if err != nil {
		return failedChildStart(spec, FailureKindExecution, "engine.child.budget_invalid", err)
	}
	deployment, err := runtime.resolveChildDeployment(ctx, spec.Deployment)
	if err != nil {
		return failedChildStart(spec, FailureKindExternal, "engine.child.deployment_unavailable", err)
	}
	if err := deployment.Descriptor().ValidateInput(spec.Input); err != nil {
		return failedChildStart(spec, FailureKindContract, "engine.child.input.invalid", err)
	}
	execution, err := startExecution(deployment.Definition(), spec.Input)
	if err != nil {
		return failedChildStart(spec, failureKindForError(err), "engine.child.start.failed", err)
	}
	state, err := captureExecution(execution)
	if err != nil {
		return failedChildStart(spec, failureKindForError(err), "engine.child.snapshot.failed", err)
	}
	execution, err = restoreExecution(deployment.Definition(), state)
	if err != nil {
		return failedChildStart(spec, failureKindForError(err), "engine.child.snapshot.unrestorable", err)
	}
	startedAt := time.Now().Round(0).UTC()
	controller := newProcessController(
		relation, deployment.Reference(), spec.Budget, spec.Capabilities, runtime.treeLimits,
		startedAt, StatusRunning,
	)
	childRuntime := newProcessRuntime(
		runtime.engine, controller, deployment, execution, state, startedAt, childLimits,
	)
	if err := runtime.engine.registerChild(controller, requestDigest); err != nil {
		if existing, exists := runtime.engine.Process(childID); exists &&
			existing.Relation() == relation && existing.DeploymentRef() == spec.Deployment &&
			existing.controller.childRequestDigest == requestDigest {
			return ChildStartResult{key: spec.Key, processID: childID, deployment: spec.Deployment}
		}
		if errors.Is(err, ErrLimitExceeded) {
			return failedChildStart(spec, FailureKindExecution, "engine.child.tree_limit", err)
		}
		return failedChildStart(spec, FailureKindContract, "engine.child.register.failed", err)
	}
	budgetCommitted = true
	go childRuntime.run(context.Background())
	return ChildStartResult{key: spec.Key, processID: childID, deployment: spec.Deployment}
}

func (runtime *processRuntime) reserveChildBudget(requested Budget) bool {
	if !runtime.budget.canAllocate(runtime.usage, runtime.reservedBudget, requested) {
		return false
	}
	reserved, ok := runtime.reservedBudget.add(requested)
	if !ok {
		return false
	}
	runtime.reservedBudget = reserved
	return true
}

func (runtime *processRuntime) releaseChildBudget(released Budget) {
	if released.Steps > runtime.reservedBudget.Steps ||
		released.Effects > runtime.reservedBudget.Effects ||
		released.Signals > runtime.reservedBudget.Signals {
		return
	}
	runtime.reservedBudget.Steps -= released.Steps
	runtime.reservedBudget.Effects -= released.Effects
	runtime.reservedBudget.Signals -= released.Signals
}

func (runtime *processRuntime) resolveChildDeployment(
	ctx context.Context,
	reference DeploymentRef,
) (Deployment, error) {
	if reference == runtime.deployment.Reference() {
		return runtime.deployment, nil
	}
	if runtime.engine.resolver == nil {
		return Deployment{}, fmt.Errorf("%w: no resolver for %s", ErrInvalidDeployment, reference.Name())
	}
	deployment, err := resolveDeployment(runtime.engine.resolver, ctx, reference)
	if err != nil {
		return Deployment{}, err
	}
	if !deployment.Valid() || deployment.Reference() != reference {
		return Deployment{}, fmt.Errorf("%w: resolver returned a different binding", ErrInvalidDeployment)
	}
	return deployment, nil
}

func resolveDeployment(
	resolver DeploymentResolver,
	ctx context.Context,
	reference DeploymentRef,
) (deployment Deployment, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			deployment = Deployment{}
			err = fmt.Errorf("deployment resolver panicked: %v", recovered)
		}
	}()
	return resolver.Resolve(context.WithoutCancel(ctx), reference)
}

func failedChildStart(spec ChildSpec, kind FailureKind, code string, cause error) ChildStartResult {
	failure, err := failureFromError(kind, code, cause)
	if err != nil {
		failure, _ = NewFailure(FailureKindContract, "engine.child.failure.invalid", "invalid child failure")
	}
	return ChildStartResult{key: spec.Key, deployment: spec.Deployment, failure: failure}
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
