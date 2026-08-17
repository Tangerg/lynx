package agentexec

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func (session *interactionSession) admitProcess(
	ctx context.Context,
	admission agent.ProcessAdmission,
) error {
	if !admission.Valid() {
		return errors.New("agentexec: Interaction received an invalid Process admission")
	}
	relation := admission.Relation()
	session.state.mu.Lock()
	deployments := session.state.deployments
	session.state.mu.Unlock()
	if deployments == nil {
		return errors.New("agentexec: Interaction deployments are unavailable")
	}
	if relation.IsRoot() {
		if admission.DeploymentRef() != deployments.root.DeploymentRef() {
			return errors.New("agentexec: Interaction root admission changed Deployment")
		}
		session.state.mu.Lock()
		defer session.state.mu.Unlock()
		if session.state.admittedProcessID.Valid() && session.state.admittedProcessID != relation.ProcessID() {
			return errors.New("agentexec: Interaction root admission identity changed")
		}
		session.state.admittedProcessID = relation.ProcessID()
		return nil
	}
	if !deployments.managedChild(admission.DeploymentRef()) {
		return errors.New("agentexec: unmanaged child Process is outside the Runtime projection contract")
	}
	parentID, _ := relation.ParentID()
	childKey, _ := relation.ChildKey()
	identity := delegateCallIdentity{parentID: parentID, childKey: childKey}
	session.state.mu.Lock()
	managed := session.state.delegateCalls[identity]
	session.state.mu.Unlock()
	if managed == nil {
		return errors.New("agentexec: child admission has no durably observed Delegate call")
	}
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if managed.target != admission.DeploymentRef() || managed.parentRelation.ProcessID() != parentID {
		return errors.New("agentexec: child admission differs from its Delegate binding")
	}
	if managed.admission.Valid() {
		if !sameManagedAdmission(managed.admission, admission) ||
			(managed.binding != (runs.ChildRunBinding{}) && managed.binding.Validate() != nil) {
			return errors.New("agentexec: repeated child admission changed immutable identity")
		}
		return nil
	}
	parent := session.executorMember(managed.parentRelation)
	started := runs.ToolCallStarted{
		CallID: managed.callID, ModelCallSequence: managed.modelCallSequence,
		ToolCallIndex: managed.toolCallIndex, SourceCallID: managed.call.ID,
		ToolName: managed.call.Name, Arguments: managed.arguments.Canonical(),
		Activity: "Delegating " + managed.input.Summary, SafetyClass: tool.SafetyClassExec,
	}
	if err := session.commitFact(ctx, parent, started); err != nil {
		return fmt.Errorf("agentexec: commit Delegate call start: %w", err)
	}
	managed.toolStarted = true
	member := runs.ExecutorMember{
		MemberID: relation.ProcessID().String(), ParentID: parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	request, receipt := runs.NewChildRunReservationRequest(admission.StartedAt())
	if err := session.sendExecutorRequest(ctx, runs.ExecutorEvent{Member: member, Payload: request}); err != nil {
		return session.failDelegateAdmission(ctx, managed, err)
	}
	binding, err := receipt.Await(ctx)
	if err != nil {
		return session.failDelegateAdmission(ctx, managed, err)
	}
	if binding.MemberID != member.MemberID || binding.ParentRunID == "" {
		return session.failDelegateAdmission(
			ctx, managed, errors.New("child Run reservation returned a different executor member"),
		)
	}
	managed.admission = admission
	managed.binding = binding
	return nil
}

func sameManagedAdmission(left, right agent.ProcessAdmission) bool {
	return left.Valid() && right.Valid() && left.Relation() == right.Relation() &&
		left.DeploymentRef() == right.DeploymentRef() && left.StartedAt().Equal(right.StartedAt())
}

func (session *interactionSession) failDelegateAdmission(
	ctx context.Context,
	managed *managedDelegateCall,
	cause error,
) error {
	if finishErr := session.finishDelegateTool(
		ctx, managed, "error: delegated worker could not start", cause,
	); finishErr != nil {
		return errors.Join(cause, finishErr)
	}
	return cause
}

func (session *interactionSession) acknowledgeProcessStartOutcome(
	ctx context.Context,
	outcome agent.ProcessStartOutcome,
) error {
	if !outcome.Valid() {
		return errors.New("agentexec: Interaction received an invalid Process start outcome")
	}
	admission := outcome.Admission()
	relation := admission.Relation()
	if relation.IsRoot() {
		if outcome.Status() != agent.ProcessStartOutcomeStatusStarted {
			return errors.New("agentexec: accepted Interaction root aborted during initialization")
		}
		return nil
	}
	parentID, _ := relation.ParentID()
	childKey, _ := relation.ChildKey()
	session.state.mu.Lock()
	managed := session.state.delegateCalls[delegateCallIdentity{parentID: parentID, childKey: childKey}]
	session.state.mu.Unlock()
	if managed == nil {
		return errors.New("agentexec: child start outcome has no Delegate admission")
	}
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if !sameManagedAdmission(managed.admission, admission) || managed.binding.MemberID == "" {
		return errors.New("agentexec: child start outcome differs from its reservation")
	}
	applicationOutcome := runs.ChildRunStartAborted
	if outcome.Status() == agent.ProcessStartOutcomeStatusStarted {
		applicationOutcome = runs.ChildRunStarted
	}
	request, receipt := runs.NewChildRunStartOutcomeRequest(managed.binding, applicationOutcome)
	member := runs.ExecutorMember{
		MemberID: relation.ProcessID().String(), ParentID: parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	if err := session.sendExecutorRequest(ctx, runs.ExecutorEvent{Member: member, Payload: request}); err != nil {
		return err
	}
	if err := receipt.Await(ctx); err != nil {
		if outcome.Status() == agent.ProcessStartOutcomeStatusStarted {
			return session.failDelegateAdmission(ctx, managed, err)
		}
		return err
	}
	if outcome.Status() == agent.ProcessStartOutcomeStatusAborted {
		failure, _ := outcome.Failure()
		return session.finishDelegateTool(
			ctx, managed,
			"error: delegated worker could not start: "+failure.Code(),
			errors.New(failure.Message()),
		)
	}
	managed.childProcessID = relation.ProcessID()
	session.state.mu.Lock()
	session.state.delegateChildren[relation.ProcessID()] = managed
	session.state.mu.Unlock()
	return nil
}
