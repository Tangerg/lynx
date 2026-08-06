package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	agent "github.com/Tangerg/lynx/agent2"
)

func (execution *execution) startFanoutWindow(consumedSignals uint32) (agent.Transition, error) {
	stage := execution.stage()
	count, err := stage.fanoutCount(execution.state.CurrentValue)
	if err != nil || count == 0 || stage.fanoutWindowSize() == 0 {
		return agent.Transition{}, errors.Join(ErrInvalidStage, err)
	}
	if execution.state.FanoutOutputs == nil {
		execution.state.FanoutOutputs = make([]*json.RawMessage, count)
	}
	start := execution.state.NextFanoutIndex
	if start >= count {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	end := start + min(stage.fanoutWindowSize(), count-start)
	inputs, err := stage.fanoutWindowInputs(start, end, execution.state.CurrentValue)
	if err != nil || len(inputs) != int(end-start) {
		return agent.Transition{}, errors.Join(ErrInvalidExecutionState, err)
	}
	window := make([]fanoutChildState, 0, end-start)
	effects := make([]agent.Effect, 0, end-start)
	for index := start; index < end; index++ {
		binding, found := stage.fanoutBinding(index)
		if !found {
			return agent.Transition{}, ErrInvalidStage
		}
		input := inputs[index-start]
		key, err := execution.fanoutChildKey(index)
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
		window = append(window, fanoutChildState{FanoutIndex: index})
		effects = append(effects, effect)
	}
	execution.state.NextFanoutIndex = end
	execution.state.ActiveFanoutWindow = window
	execution.state.Phase = phaseAwaitingFanoutStarts
	return agent.Continue(consumedSignals, effects...)
}

func (execution *execution) acceptFanoutStarts(signals []agent.Signal) (agent.Transition, error) {
	window := execution.state.ActiveFanoutWindow
	if len(signals) < len(window) {
		return agent.Transition{}, fmt.Errorf("%w: fan-out child starts require one settlement Signal per member", ErrInvalidProtocol)
	}
	childIDs := make([]agent.ProcessID, 0, len(window))
	for offset := range window {
		index := window[offset].FanoutIndex
		binding, found := execution.stage().fanoutBinding(index)
		memberID, identified := execution.stage().fanoutMemberID(index)
		result, err := agent.ParseChildStartResult(signals[offset])
		key, keyErr := execution.fanoutChildKey(index)
		if err != nil || keyErr != nil || !found || !identified ||
			result.Key() != key || result.DeploymentRef() != binding.deploymentRef {
			return agent.Transition{}, fmt.Errorf(
				"%w: %s Stage %q member %q start result mismatch",
				ErrInvalidProtocol, execution.stage().kind, execution.stage().id, memberID,
			)
		}
		if failure, failed := result.Failure(); failed {
			attributed, err := agent.NewFailure(
				failure.Kind(), execution.stage().fanoutFailureCode("start_failed"),
				execution.fanoutFailureMessage(index, "failed to start: "+failure.Code()),
			)
			if err != nil {
				return agent.Transition{}, err
			}
			execution.state.ActiveFanoutWindow[offset].Failure = &attributed
			continue
		}
		processID, started := result.ProcessID()
		if !started {
			return agent.Transition{}, fmt.Errorf("%w: fan-out child-start result has no Process", ErrInvalidProtocol)
		}
		execution.state.ActiveFanoutWindow[offset].ChildProcessID = &processID
		childIDs = append(childIDs, processID)
	}
	consumedSignals := uint32(len(window))
	if len(childIDs) == 0 {
		return agent.Fail(consumedSignals, execution.lowestFanoutFailure())
	}
	waitKey, err := execution.fanoutWaitKey()
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: childIDs, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseAwaitingFanoutWaitOpen
	return agent.Continue(consumedSignals, effect)
}

func (execution *execution) acceptFanoutWaitOpen(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, fmt.Errorf("%w: fan-out wait opening requires its settlement Signal", ErrInvalidProtocol)
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	wantKey, keyErr := execution.fanoutWaitKey()
	spec := opened.Spec()
	wantChildren := execution.fanoutStartedChildren()
	if err != nil || keyErr != nil || spec.Key != wantKey || spec.Condition != agent.AllChildren() ||
		!slices.Equal(spec.Children, wantChildren) {
		return agent.Transition{}, fmt.Errorf("%w: fan-out child wait does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	waitID := opened.WaitID()
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingFanout
	return agent.Wait(1, waitID)
}

func (execution *execution) acceptFanoutCompletion(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 || execution.state.WaitID == nil {
		return agent.Transition{}, fmt.Errorf("%w: fan-out completion requires one active child wait Signal", ErrInvalidProtocol)
	}
	completed, err := agent.ParseChildrenCompleted(signals[0])
	wantKey, keyErr := execution.fanoutWaitKey()
	if err != nil || keyErr != nil || completed.WaitID() != *execution.state.WaitID || completed.Key() != wantKey {
		return agent.Transition{}, fmt.Errorf("%w: fan-out completion does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	outcomes := completed.Outcomes()
	if len(outcomes) != len(execution.fanoutStartedChildren()) {
		return agent.Transition{}, fmt.Errorf("%w: fan-out completion outcome count mismatch", ErrInvalidProtocol)
	}
	windowOutputs := make(map[uint32]json.RawMessage, len(outcomes))
	outcomeIndex := 0
	for offset := range execution.state.ActiveFanoutWindow {
		child := &execution.state.ActiveFanoutWindow[offset]
		if child.ChildProcessID == nil {
			continue
		}
		outcome := outcomes[outcomeIndex]
		outcomeIndex++
		wantChildKey, err := execution.fanoutChildKey(child.FanoutIndex)
		if err != nil || outcome.Key() != wantChildKey || outcome.Result().ProcessID() != *child.ChildProcessID {
			return agent.Transition{}, fmt.Errorf("%w: fan-out member outcome mismatch", ErrInvalidProtocol)
		}
		failure, output, err := execution.fanoutOutcome(child.FanoutIndex, outcome.Result())
		if err != nil {
			return agent.Transition{}, err
		}
		if failure != nil {
			child.Failure = failure
			continue
		}
		windowOutputs[child.FanoutIndex] = output
	}
	if failure := execution.lowestFanoutFailure(); failure.Valid() {
		return agent.Fail(1, failure)
	}
	for index, output := range windowOutputs {
		owned := output
		execution.state.FanoutOutputs[index] = &owned
	}
	execution.state.WaitID = nil
	execution.state.ActiveFanoutWindow = nil
	count, err := execution.stage().fanoutCount(execution.state.CurrentValue)
	if err != nil {
		return agent.Transition{}, err
	}
	if execution.state.NextFanoutIndex < count {
		return execution.startFanoutWindow(1)
	}
	outputs := make([]json.RawMessage, len(execution.state.FanoutOutputs))
	for index, output := range execution.state.FanoutOutputs {
		if output == nil {
			return agent.Transition{}, ErrInvalidExecutionState
		}
		outputs[index] = *output
	}
	value, err := execution.stage().fanoutComplete(outputs)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.CurrentValue = value
	execution.state.Phase = phaseReady
	return execution.finishStage(1)
}

func (execution *execution) fanoutOutcome(
	index uint32,
	result agent.Result,
) (*agent.Failure, json.RawMessage, error) {
	if result.Status() != agent.StatusCompleted {
		code := execution.stage().fanoutFailureCode("not_completed")
		message := execution.fanoutFailureMessage(index, "terminated with status "+result.Status().String())
		if childFailure, failed := result.Termination().Failure(); failed {
			code = execution.stage().fanoutFailureCode("failed")
			message = execution.fanoutFailureMessage(index, "failed: "+childFailure.Code())
		}
		failure, err := agent.NewFailure(agent.FailureKindExternal, code, message)
		return &failure, nil, err
	}
	output, present := result.Output()
	if !present {
		failure, err := agent.NewFailure(
			agent.FailureKindContract, execution.stage().fanoutFailureCode("output_missing"),
			execution.fanoutFailureMessage(index, "returned no Output"),
		)
		return &failure, nil, err
	}
	if err := execution.stage().fanoutOutputSchema().ValidateOutput(output); err != nil {
		failure, failureErr := agent.NewFailure(
			agent.FailureKindContract, execution.stage().fanoutFailureCode("output_invalid"),
			execution.fanoutFailureMessage(index, "violated its Output contract"),
		)
		return &failure, nil, failureErr
	}
	return nil, output.JSON(), nil
}

func (execution *execution) fanoutFailureMessage(index uint32, diagnostic string) string {
	return execution.stage().kind.String() + " Stage " + execution.stage().id + " " +
		execution.stage().fanoutMemberLabel(index) + " " + diagnostic
}

func (execution *execution) lowestFanoutFailure() agent.Failure {
	for _, child := range execution.state.ActiveFanoutWindow {
		if child.Failure != nil && child.Failure.Valid() {
			return *child.Failure
		}
	}
	return agent.Failure{}
}

func (execution *execution) fanoutStartedChildren() []agent.ProcessID {
	children := make([]agent.ProcessID, 0, len(execution.state.ActiveFanoutWindow))
	for _, child := range execution.state.ActiveFanoutWindow {
		if child.ChildProcessID != nil {
			children = append(children, *child.ChildProcessID)
		}
	}
	return children
}

func (execution *execution) fanoutChildKey(index uint32) (agent.ChildKey, error) {
	memberID, found := execution.stage().fanoutMemberID(index)
	if !found {
		return agent.ChildKey{}, ErrInvalidExecutionState
	}
	return workflowChildKey(execution.stage().kind.String(), execution.stage().id, memberID)
}

func (execution *execution) fanoutWaitKey() (agent.WaitKey, error) {
	windowStart := execution.state.NextFanoutIndex - uint32(len(execution.state.ActiveFanoutWindow))
	return workflowWaitKey(
		execution.stage().kind.String(), execution.stage().id,
		strconv.FormatUint(uint64(windowStart), 10),
	)
}

func (execution *execution) clearFanout() {
	execution.state.NextFanoutIndex = 0
	execution.state.ActiveFanoutWindow = nil
	execution.state.FanoutOutputs = nil
}
