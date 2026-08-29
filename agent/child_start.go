package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type childStartPreparation struct {
	plan   *childStartPlan
	result ChildStartResult
}

type childStartPlan struct {
	engine           *Engine
	parentDeployment Deployment
	spec             ChildSpec
	childID          ProcessID
	relation         ProcessRelation
	limits           Limits
	treeLimits       TreeLimits
	requestDigest    Digest
}

type childStartJobResult struct {
	result     ChildStartResult
	deployment Deployment
	execution  Execution
	state      ExecutionState
	startedAt  time.Time
	admission  ProcessAdmission
	failure    Failure
	admitted   bool
}

func (r childStartJobResult) started() bool {
	_, failed := r.result.Failure()
	return !failed && r.deployment.Valid() && r.execution != nil &&
		r.state.Valid() && !r.startedAt.IsZero()
}

func (p *processState) prepareChildStart(
	effectID EffectID,
	spec ChildSpec,
) childStartPreparation {
	if !spec.Valid() || !p.controller.relation.Valid() {
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindContract, "engine.child.request.invalid", ErrInvalidChildStart,
		)}
	}
	childID := deriveChildProcessID(effectID)
	relation := childProcessRelation(childID, p.controller.relation, spec.Key)
	requestDigest, err := childSpecDigest(spec)
	if err != nil {
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindContract, "engine.child.request.invalid", err,
		)}
	}
	if existing, exists := p.engine.Process(childID); exists {
		if existing.Relation() == relation && existing.DeploymentRef() == spec.DeploymentRef &&
			existing.controller.childRequestDigest == requestDigest {
			return childStartPreparation{result: ChildStartResult{
				key: spec.Key, processID: childID, deploymentRef: spec.DeploymentRef,
			}}
		}
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindContract, "engine.child.identity_conflict", ErrInvalidChildStart,
		)}
	}
	if !p.capabilities.Allows(spec.Capabilities) {
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindContract, "engine.child.capability_escalation", ErrInvalidCapability,
		)}
	}
	if !p.reserveProvisionalChildBudget(spec.Budget) {
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindExecution, "engine.child.budget_exhausted", ErrResourceLimitExceeded,
		)}
	}
	childLimits, err := limitsFromBudget(p.limits, spec.Budget)
	if err != nil {
		p.releaseProvisionalChildBudget(spec.Budget)
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindExecution, "engine.child.budget_invalid", err,
		)}
	}
	if reserveProcessStartErr := p.engine.reserveProcessStart(
		relation, spec.DeploymentRef, p.treeLimits, requestDigest,
	); reserveProcessStartErr != nil {
		p.releaseProvisionalChildBudget(spec.Budget)
		if errors.Is(reserveProcessStartErr, ErrResourceLimitExceeded) {
			return childStartPreparation{result: failedChildStart(
				spec, FailureKindExecution, "engine.child.tree_limit", reserveProcessStartErr,
			)}
		}
		if errors.Is(reserveProcessStartErr, ErrEngineClosed) {
			return childStartPreparation{result: failedChildStart(
				spec, FailureKindExternal, "engine.child.start.unavailable", reserveProcessStartErr,
			)}
		}
		return childStartPreparation{result: failedChildStart(
			spec, FailureKindContract, "engine.child.identity_conflict", reserveProcessStartErr,
		)}
	}
	return childStartPreparation{plan: &childStartPlan{
		engine: p.engine, parentDeployment: p.deployment,
		spec: spec, childID: childID, relation: relation,
		limits: childLimits, treeLimits: p.treeLimits,
		requestDigest: requestDigest,
	}}
}

func (p *childStartPlan) execute(ctx context.Context) childStartJobResult {
	deployment, resolveErr := p.resolveDeployment()
	if resolveErr != nil {
		return childStartJobResult{result: failedChildStart(
			p.spec, FailureKindExternal, "engine.child.deployment_unavailable", resolveErr,
		)}
	}
	if validateErr := deployment.Descriptor().ValidateInput(p.spec.Input); validateErr != nil {
		return childStartJobResult{result: failedChildStart(
			p.spec, FailureKindContract, "engine.child.input.invalid", validateErr,
		)}
	}
	admission := newProcessAdmission(p.relation, deployment, p.spec.Budget, p.spec.Capabilities)
	if admissionErr := requestProcessAdmission(ctx, p.engine.admitter, admission); admissionErr != nil {
		return childStartJobResult{result: failedChildStart(
			p.spec, FailureKindExternal, "engine.child.admission.rejected", admissionErr,
		)}
	}
	startedAt := time.Now().Round(0).UTC()
	execution, state, failure, err := initializeExecution(deployment.Definition(), p.spec.Input)
	if err != nil {
		return childStartJobResult{
			result:    failedChildStart(p.spec, failure.Kind(), failure.Code(), err),
			admission: admission, failure: failure, admitted: true,
		}
	}
	return childStartJobResult{
		result: ChildStartResult{
			key: p.spec.Key, processID: p.childID, deploymentRef: p.spec.DeploymentRef,
		},
		deployment: deployment, execution: execution, state: state, startedAt: startedAt,
		admission: admission, admitted: true,
	}
}

func (p *processState) reserveProvisionalChildBudget(requested Budget) bool {
	if p.provisionalChildBudget.Valid() ||
		!p.budget.canAllocate(p.usage, p.effectiveReservedBudget(), requested) {
		return false
	}
	p.provisionalChildBudget = requested
	return true
}

func (p *processState) commitProvisionalChildBudget(requested Budget) error {
	if !requested.Valid() || p.provisionalChildBudget != requested {
		return ErrResourceLimitExceeded
	}
	reserved, ok := p.reservedBudget.add(requested)
	if !ok {
		return ErrResourceLimitExceeded
	}
	p.reservedBudget = reserved
	p.provisionalChildBudget = Budget{}
	return nil
}

func (p *processState) releaseProvisionalChildBudget(requested Budget) {
	if p.provisionalChildBudget == requested {
		p.provisionalChildBudget = Budget{}
	}
}

func (p *processState) releaseCommittedChildBudget(released Budget) {
	if released.Steps > p.reservedBudget.Steps ||
		released.Effects > p.reservedBudget.Effects ||
		released.Signals > p.reservedBudget.Signals {
		return
	}
	p.reservedBudget.Steps -= released.Steps
	p.reservedBudget.Effects -= released.Effects
	p.reservedBudget.Signals -= released.Signals
}

func (p *processState) effectiveReservedBudget() Budget {
	reserved, ok := p.reservedBudget.add(p.provisionalChildBudget)
	if !ok {
		panic("agent: Process resource reservation overflow")
	}
	return reserved
}

func (p *childStartPlan) resolveDeployment() (Deployment, error) {
	reference := p.spec.DeploymentRef
	if reference == p.parentDeployment.DeploymentRef() {
		return p.parentDeployment, nil
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
	id, err := ParseProcessID(processIDPrefix + digest.hex())
	if err != nil {
		panic(err)
	}
	return id
}
