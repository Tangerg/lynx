package workflow

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

type execution struct {
	definition *Definition
	state      executionState
}

func (execution *execution) Step(_ context.Context, signals []agent.Signal) (agent.Transition, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	if err := execution.state.validate(execution.definition); err != nil {
		return agent.Transition{}, err
	}
	switch execution.state.Phase {
	case phaseReady:
		return execution.advance(signals)
	case phaseAwaitingChildStart:
		return execution.acceptChildStart(signals)
	case phaseAwaitingChildWaitOpen:
		return execution.acceptChildWaitOpen(signals)
	case phaseWaitingChild:
		return execution.acceptChildCompletion(signals)
	case phaseCompleted:
		return agent.Transition{}, fmt.Errorf("%w: completed Execution cannot advance", ErrInvalidProtocol)
	default:
		return agent.Transition{}, ErrInvalidExecutionState
	}
}

func (execution *execution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.ExecutionState{}, ErrInvalidExecutionState
	}
	if err := execution.state.validate(execution.definition); err != nil {
		return agent.ExecutionState{}, err
	}
	return encodeExecutionState(execution.state)
}

func (execution *execution) advance(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 0 {
		return agent.Transition{}, fmt.Errorf("%w: Stage %q does not accept unsolicited Signals", ErrInvalidProtocol, execution.stage().id)
	}
	stage := execution.stage()
	switch stage.kind {
	case StageKindTransform:
		value, err := stage.transform(execution.state.Value)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.Value = value
		return execution.finishStage(0)
	case StageKindCall:
		input, err := agent.ParseInput(execution.state.Value)
		if err != nil {
			return agent.Transition{}, err
		}
		key, err := execution.childKey()
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.StartChild(agent.ChildSpec{
			Key: key, Deployment: stage.call.deployment, Input: input,
			Budget: stage.call.budget, Capabilities: stage.call.capabilities,
		})
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.Phase = phaseAwaitingChildStart
		return agent.Continue(0, effect)
	default:
		return agent.Transition{}, ErrInvalidStage
	}
}

func (execution *execution) acceptChildStart(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, errors.New("workflow: child start requires its settlement Signal")
	}
	result, err := agent.ParseChildStartResult(signals[0])
	key, keyErr := execution.childKey()
	stage := execution.stage()
	if err != nil || keyErr != nil || result.Key() != key || result.DeploymentRef() != stage.call.deployment {
		return agent.Transition{}, fmt.Errorf("%w: child-start result does not match Stage %q", ErrInvalidProtocol, stage.id)
	}
	if failure, failed := result.Failure(); failed {
		return execution.fail(
			1, "workflow.call.start_failed",
			"Child Process start failed for Stage "+stage.id+": "+failure.Code(),
			failure.Kind(),
		)
	}
	childID, started := result.ProcessID()
	if !started {
		return agent.Transition{}, fmt.Errorf("%w: child-start result has no Process", ErrInvalidProtocol)
	}
	waitKey, err := execution.waitKey()
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: []agent.ProcessID{childID}, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.ChildProcessID = &childID
	execution.state.Phase = phaseAwaitingChildWaitOpen
	return agent.Continue(1, effect)
}

func (execution *execution) acceptChildWaitOpen(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 || execution.state.ChildProcessID == nil {
		return agent.Transition{}, errors.New("workflow: child wait opening requires its settlement Signal")
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	wantKey, keyErr := execution.waitKey()
	spec := opened.Spec()
	if err != nil || keyErr != nil || spec.Key != wantKey || len(spec.Children) != 1 ||
		spec.Children[0] != *execution.state.ChildProcessID || spec.Condition != agent.AllChildren() {
		return agent.Transition{}, fmt.Errorf("%w: child wait opening does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	waitID := opened.WaitID()
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingChild
	return agent.Wait(1, waitID)
}

func (execution *execution) acceptChildCompletion(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 || execution.state.ChildProcessID == nil || execution.state.WaitID == nil {
		return agent.Transition{}, errors.New("workflow: child completion requires one active child wait Signal")
	}
	completed, err := agent.ParseChildrenCompleted(signals[0])
	wantWaitKey, keyErr := execution.waitKey()
	if err != nil || keyErr != nil || completed.WaitID() != *execution.state.WaitID || completed.Key() != wantWaitKey {
		return agent.Transition{}, fmt.Errorf("%w: child completion does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	outcomes := completed.Outcomes()
	wantChildKey, err := execution.childKey()
	if err != nil || len(outcomes) != 1 || outcomes[0].Key() != wantChildKey ||
		outcomes[0].Result().ProcessID() != *execution.state.ChildProcessID {
		return agent.Transition{}, fmt.Errorf("%w: child outcome does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	result := outcomes[0].Result()
	if result.Status() != agent.StatusCompleted {
		if failure, failed := result.Termination().Failure(); failed {
			return execution.failExternal(
				1, "workflow.call.child_failed",
				"Child Process failed for Stage "+execution.stage().id+": "+failure.Code(),
			)
		}
		return execution.failExternal(
			1,
			"workflow.call.child_not_completed",
			"Child Process for Stage "+execution.stage().id+" terminated with status "+result.Status().String(),
		)
	}
	output, present := result.Output()
	if !present {
		return execution.failContract(1, "workflow.call.output_missing", "Completed child Process returned no Output")
	}
	if err := execution.stage().outputSchema.ValidateOutput(output); err != nil {
		return execution.failContract(1, "workflow.call.output_invalid", "Child Process Output violated the Stage contract")
	}
	execution.state.Value = output.JSON()
	execution.state.ChildProcessID = nil
	execution.state.WaitID = nil
	execution.state.Phase = phaseReady
	return execution.finishStage(1)
}

func (execution *execution) finishStage(consumed uint32) (agent.Transition, error) {
	execution.state.Stage++
	if execution.state.Stage < uint32(len(execution.definition.stages)) {
		execution.state.Phase = phaseReady
		return agent.Continue(consumed)
	}
	output, err := agent.ParseOutput(execution.state.Value)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseCompleted
	return agent.Complete(consumed, output)
}

func (execution *execution) failContract(consumed uint32, code, message string) (agent.Transition, error) {
	return execution.fail(consumed, code, message, agent.FailureKindContract)
}

func (execution *execution) failExternal(consumed uint32, code, message string) (agent.Transition, error) {
	return execution.fail(consumed, code, message, agent.FailureKindExternal)
}

func (*execution) fail(
	consumed uint32,
	code string,
	message string,
	kind agent.FailureKind,
) (agent.Transition, error) {
	failure, err := agent.NewFailure(kind, code, message)
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Fail(consumed, failure)
}

func (execution *execution) stage() Stage {
	return execution.definition.stages[execution.state.Stage]
}

func (execution *execution) childKey() (agent.ChildKey, error) {
	return agent.ParseChildKey("workflow.stage." + execution.stage().id)
}

func (execution *execution) waitKey() (agent.WaitKey, error) {
	return agent.ParseWaitKey("workflow.stage." + execution.stage().id)
}

var _ agent.Execution = (*execution)(nil)
