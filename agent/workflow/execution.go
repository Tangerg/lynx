package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/lynx/agent"
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
	case phaseAwaitingFanoutStarts:
		return execution.acceptFanoutStarts(signals)
	case phaseAwaitingFanoutWaitOpen:
		return execution.acceptFanoutWaitOpen(signals)
	case phaseWaitingFanout:
		return execution.acceptFanoutCompletion(signals)
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
	case stageKindTransform:
		value, err := stage.transform(execution.state.CurrentValue)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.CurrentValue = value
		return execution.finishStage(0)
	case stageKindCall:
		return execution.startSingleChild(0, stage.call)
	case stageKindSwitch:
		selected, err := stage.switcher.selectCase(execution.state.CurrentValue)
		if err != nil {
			var unknown unknownSwitchCaseError
			if errors.As(err, &unknown) {
				return execution.failContract(
					0, "workflow.switch.case_unknown",
					"Switch Stage "+stage.id+" selected an undeclared case",
				)
			}
			return agent.Transition{}, err
		}
		binding, found := stage.switcher.binding(selected)
		if !found {
			return agent.Transition{}, ErrInvalidStage
		}
		execution.state.SelectedCaseID = selected
		return execution.startSingleChild(0, binding)
	case stageKindFork:
		return execution.startFanoutWindow(0)
	case stageKindMap:
		count, err := stage.fanoutCount(execution.state.CurrentValue)
		if err != nil {
			var exceeded mapItemLimitExceededError
			if errors.As(err, &exceeded) {
				return execution.failContract(
					0, "workflow.map.item_limit_exceeded",
					"Map Stage "+stage.id+" input exceeds its configured item limit",
				)
			}
			return agent.Transition{}, err
		}
		if count > 0 {
			return execution.startFanoutWindow(0)
		}
		value, err := stage.fanoutComplete([]json.RawMessage{})
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.CurrentValue = value
		return execution.finishStage(0)
	case stageKindLoop:
		execution.state.LoopIteration = 1
		return execution.startSingleChild(0, stage.loop.binding)
	default:
		return agent.Transition{}, ErrInvalidStage
	}
}

func (execution *execution) startSingleChild(consumedSignals uint32, binding childBinding) (agent.Transition, error) {
	input, err := agent.ParseInput(execution.state.CurrentValue)
	if err != nil {
		return agent.Transition{}, err
	}
	key, err := execution.childKey()
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.StartChild(agent.ChildSpec{
		Key: key, DeploymentRef: binding.deploymentRef, Input: input,
		Budget: binding.budget, Capabilities: binding.capabilities,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseAwaitingChildStart
	return agent.Continue(consumedSignals, effect)
}

func (execution *execution) singleChildBinding() (childBinding, bool) {
	stage := execution.stage()
	switch stage.kind {
	case stageKindCall:
		return stage.call, true
	case stageKindSwitch:
		return stage.switcher.binding(execution.state.SelectedCaseID)
	case stageKindLoop:
		return stage.loop.binding, stage.loop.valid()
	default:
		return childBinding{}, false
	}
}

func (execution *execution) clearSingleChild() {
	execution.state.SelectedCaseID = ""
	execution.state.ChildProcessID = nil
	execution.state.WaitID = nil
}

func (execution *execution) singleChildID() string {
	if execution.stage().kind == stageKindSwitch {
		return execution.stage().id + ".case." + execution.state.SelectedCaseID
	}
	if execution.stage().kind == stageKindLoop {
		return execution.stage().id + ".iteration." + strconv.FormatUint(uint64(execution.state.LoopIteration), 10)
	}
	return execution.stage().id
}

func (execution *execution) acceptChildStart(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, fmt.Errorf("%w: child start requires its settlement Signal", ErrInvalidProtocol)
	}
	result, err := agent.ParseChildStartResult(signals[0])
	key, keyErr := execution.childKey()
	binding, bound := execution.singleChildBinding()
	stage := execution.stage()
	if err != nil || keyErr != nil || !bound || result.Key() != key || result.DeploymentRef() != binding.deploymentRef {
		return agent.Transition{}, fmt.Errorf("%w: child-start result does not match Stage %q", ErrInvalidProtocol, stage.id)
	}
	if failure, failed := result.Failure(); failed {
		return execution.fail(
			1, "workflow."+stage.kind.String()+".start_failed",
			"Child Process start failed for Stage "+execution.singleChildID()+": "+failure.Code(),
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
		return agent.Transition{}, fmt.Errorf("%w: child wait opening requires its settlement Signal", ErrInvalidProtocol)
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
		return agent.Transition{}, fmt.Errorf("%w: child completion requires one active child wait Signal", ErrInvalidProtocol)
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
				1, "workflow."+execution.stage().kind.String()+".child_failed",
				"Child Process failed for Stage "+execution.singleChildID()+": "+failure.Code(),
			)
		}
		return execution.failExternal(
			1,
			"workflow."+execution.stage().kind.String()+".child_not_completed",
			"Child Process for Stage "+execution.singleChildID()+" terminated with status "+result.Status().String(),
		)
	}
	output, present := result.Output()
	if !present {
		return execution.failContract(1, "workflow."+execution.stage().kind.String()+".output_missing", "Completed child Process returned no Output")
	}
	if err := execution.singleChildOutputSchema().ValidateOutput(output); err != nil {
		return execution.failContract(1, "workflow."+execution.stage().kind.String()+".output_invalid", "Child Process Output violated the Stage contract")
	}
	if execution.stage().kind == stageKindLoop {
		return execution.finishLoopIteration(1, output)
	}
	execution.state.CurrentValue = output.JSON()
	execution.clearSingleChild()
	execution.state.Phase = phaseReady
	return execution.finishStage(1)
}

func (execution *execution) finishStage(consumedSignals uint32) (agent.Transition, error) {
	execution.clearSingleChild()
	execution.clearFanout()
	execution.state.LoopIteration = 0
	execution.state.StageIndex++
	if execution.state.StageIndex < uint32(len(execution.definition.stages)) {
		execution.state.Phase = phaseReady
		return agent.Continue(consumedSignals)
	}
	output, err := agent.ParseOutput(execution.state.CurrentValue)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseCompleted
	return agent.Complete(consumedSignals, output)
}

func (execution *execution) failContract(consumedSignals uint32, code, message string) (agent.Transition, error) {
	return execution.fail(consumedSignals, code, message, agent.FailureKindContract)
}

func (execution *execution) failExternal(consumedSignals uint32, code, message string) (agent.Transition, error) {
	return execution.fail(consumedSignals, code, message, agent.FailureKindExternal)
}

func (*execution) fail(
	consumedSignals uint32,
	code string,
	message string,
	kind agent.FailureKind,
) (agent.Transition, error) {
	failure, err := agent.NewFailure(kind, code, message)
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Fail(consumedSignals, failure)
}

func (execution *execution) stage() Stage {
	return execution.definition.stages[execution.state.StageIndex]
}

func (execution *execution) childKey() (agent.ChildKey, error) {
	return workflowChildKey(
		"single", execution.stage().id, execution.state.SelectedCaseID,
		strconv.FormatUint(uint64(execution.state.LoopIteration), 10),
	)
}

func (execution *execution) waitKey() (agent.WaitKey, error) {
	return workflowWaitKey(
		"single", execution.stage().id, execution.state.SelectedCaseID,
		strconv.FormatUint(uint64(execution.state.LoopIteration), 10),
	)
}

func (execution *execution) singleChildOutputSchema() agent.Schema {
	if execution.stage().kind == stageKindLoop {
		return execution.stage().loop.valueSchema
	}
	return execution.stage().outputSchema
}

func (execution *execution) finishLoopIteration(
	consumedSignals uint32,
	output agent.Output,
) (agent.Transition, error) {
	stage := execution.stage()
	satisfied, err := stage.loop.predicate(output.JSON())
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.CurrentValue = output.JSON()
	if satisfied || execution.state.LoopIteration == stage.loop.maxIterations {
		value, err := stage.loop.result(execution.state.CurrentValue, execution.state.LoopIteration, satisfied)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.CurrentValue = value
		execution.clearSingleChild()
		execution.state.Phase = phaseReady
		return execution.finishStage(consumedSignals)
	}
	execution.clearSingleChild()
	execution.state.LoopIteration++
	return execution.startSingleChild(consumedSignals, stage.loop.binding)
}

var _ agent.Execution = (*execution)(nil)
