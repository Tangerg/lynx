package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	agent "github.com/Tangerg/lynx/agent2"
)

func (execution *execution) startForkWindow(consumed uint32) (agent.Transition, error) {
	stage := execution.stage()
	if stage.kind != StageKindFork || !stage.fork.valid() {
		return agent.Transition{}, ErrInvalidStage
	}
	if execution.state.ForkOutputs == nil {
		execution.state.ForkOutputs = make([]*json.RawMessage, len(stage.fork.branches))
	}
	start := execution.state.ForkNext
	if start >= uint32(len(stage.fork.branches)) {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	total := uint32(len(stage.fork.branches))
	end := start + min(stage.fork.concurrency, total-start)
	input, err := agent.ParseInput(execution.state.Value)
	if err != nil {
		return agent.Transition{}, err
	}
	window := make([]forkChildState, 0, end-start)
	effects := make([]agent.Effect, 0, end-start)
	for branchIndex := start; branchIndex < end; branchIndex++ {
		branch := stage.fork.branches[branchIndex]
		key, err := execution.forkChildKey(branchIndex)
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.StartChild(agent.ChildSpec{
			Key: key, Deployment: branch.binding.deployment, Input: input,
			Budget: branch.binding.budget, Capabilities: branch.binding.capabilities,
		})
		if err != nil {
			return agent.Transition{}, err
		}
		window = append(window, forkChildState{Branch: branchIndex})
		effects = append(effects, effect)
	}
	execution.state.ForkNext = end
	execution.state.ForkWindow = window
	execution.state.Phase = phaseAwaitingForkStarts
	return agent.Continue(consumed, effects...)
}

func (execution *execution) acceptForkStarts(signals []agent.Signal) (agent.Transition, error) {
	window := execution.state.ForkWindow
	if len(signals) < len(window) {
		return agent.Transition{}, errors.New("workflow: Fork child starts require one settlement Signal per branch")
	}
	childIDs := make([]agent.ProcessID, 0, len(window))
	for offset := range window {
		branchIndex := window[offset].Branch
		branch := execution.stage().fork.branches[branchIndex]
		result, err := agent.ParseChildStartResult(signals[offset])
		key, keyErr := execution.forkChildKey(branchIndex)
		if err != nil || keyErr != nil || result.Key() != key || result.DeploymentRef() != branch.binding.deployment {
			return agent.Transition{}, fmt.Errorf(
				"%w: Fork Stage %q branch %q start result mismatch",
				ErrInvalidProtocol, execution.stage().id, branch.id,
			)
		}
		if failure, failed := result.Failure(); failed {
			attributed, err := agent.NewFailure(
				failure.Kind(), "workflow.fork.branch_start_failed",
				"Fork Stage "+execution.stage().id+" branch "+branch.id+" failed to start: "+failure.Code(),
			)
			if err != nil {
				return agent.Transition{}, err
			}
			execution.state.ForkWindow[offset].Failure = &attributed
			continue
		}
		processID, started := result.ProcessID()
		if !started {
			return agent.Transition{}, fmt.Errorf("%w: Fork child-start result has no Process", ErrInvalidProtocol)
		}
		execution.state.ForkWindow[offset].ProcessID = &processID
		childIDs = append(childIDs, processID)
	}
	consumed := uint32(len(window))
	if len(childIDs) == 0 {
		failure := execution.lowestForkFailure()
		return agent.Fail(consumed, failure)
	}
	waitKey, err := execution.forkWaitKey()
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: childIDs, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseAwaitingForkWaitOpen
	return agent.Continue(consumed, effect)
}

func (execution *execution) acceptForkWaitOpen(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, errors.New("workflow: Fork wait opening requires its settlement Signal")
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	wantKey, keyErr := execution.forkWaitKey()
	spec := opened.Spec()
	wantChildren := execution.forkStartedChildren()
	if err != nil || keyErr != nil || spec.Key != wantKey || spec.Condition != agent.AllChildren() ||
		!slices.Equal(spec.Children, wantChildren) {
		return agent.Transition{}, fmt.Errorf("%w: Fork child wait does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	waitID := opened.WaitID()
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingFork
	return agent.Wait(1, waitID)
}

func (execution *execution) acceptForkCompletion(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 || execution.state.WaitID == nil {
		return agent.Transition{}, errors.New("workflow: Fork completion requires one active child wait Signal")
	}
	completed, err := agent.ParseChildrenCompleted(signals[0])
	wantKey, keyErr := execution.forkWaitKey()
	if err != nil || keyErr != nil || completed.WaitID() != *execution.state.WaitID || completed.Key() != wantKey {
		return agent.Transition{}, fmt.Errorf("%w: Fork completion does not match Stage %q", ErrInvalidProtocol, execution.stage().id)
	}
	outcomes := completed.Outcomes()
	if len(outcomes) != len(execution.forkStartedChildren()) {
		return agent.Transition{}, fmt.Errorf("%w: Fork completion outcome count mismatch", ErrInvalidProtocol)
	}
	windowOutputs := make(map[uint32]json.RawMessage, len(outcomes))
	outcomeIndex := 0
	for offset := range execution.state.ForkWindow {
		child := &execution.state.ForkWindow[offset]
		if child.ProcessID == nil {
			continue
		}
		outcome := outcomes[outcomeIndex]
		outcomeIndex++
		wantChildKey, err := execution.forkChildKey(child.Branch)
		if err != nil || outcome.Key() != wantChildKey || outcome.Result().ProcessID() != *child.ProcessID {
			return agent.Transition{}, fmt.Errorf("%w: Fork branch outcome mismatch", ErrInvalidProtocol)
		}
		failure, output, err := execution.forkOutcome(child.Branch, outcome.Result())
		if err != nil {
			return agent.Transition{}, err
		}
		if failure != nil {
			child.Failure = failure
			continue
		}
		windowOutputs[child.Branch] = output
	}
	if failure := execution.lowestForkFailure(); failure.Valid() {
		return agent.Fail(1, failure)
	}
	for branch, output := range windowOutputs {
		owned := output
		execution.state.ForkOutputs[branch] = &owned
	}
	execution.state.WaitID = nil
	execution.state.ForkWindow = nil
	if execution.state.ForkNext < uint32(len(execution.stage().fork.branches)) {
		return execution.startForkWindow(1)
	}
	outputs := make([]json.RawMessage, len(execution.state.ForkOutputs))
	for index, output := range execution.state.ForkOutputs {
		if output == nil {
			return agent.Transition{}, ErrInvalidExecutionState
		}
		outputs[index] = *output
	}
	value, err := execution.stage().fork.reduce(outputs)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Value = value
	execution.state.Phase = phaseReady
	return execution.finishStage(1)
}

func (execution *execution) forkOutcome(
	branchIndex uint32,
	result agent.Result,
) (*agent.Failure, json.RawMessage, error) {
	branch := execution.stage().fork.branches[branchIndex]
	if result.Status() != agent.StatusCompleted {
		code := "workflow.fork.branch_not_completed"
		message := "Fork Stage " + execution.stage().id + " branch " + branch.id +
			" terminated with status " + result.Status().String()
		if childFailure, failed := result.Termination().Failure(); failed {
			code = "workflow.fork.branch_failed"
			message = "Fork Stage " + execution.stage().id + " branch " + branch.id +
				" failed: " + childFailure.Code()
		}
		failure, err := agent.NewFailure(agent.FailureKindExternal, code, message)
		return &failure, nil, err
	}
	output, present := result.Output()
	if !present {
		failure, err := agent.NewFailure(
			agent.FailureKindContract, "workflow.fork.output_missing",
			"Fork Stage "+execution.stage().id+" branch "+branch.id+" returned no Output",
		)
		return &failure, nil, err
	}
	if err := execution.stage().fork.branchSchema.ValidateOutput(output); err != nil {
		failure, failureErr := agent.NewFailure(
			agent.FailureKindContract, "workflow.fork.output_invalid",
			"Fork Stage "+execution.stage().id+" branch "+branch.id+" violated its Output contract",
		)
		return &failure, nil, failureErr
	}
	return nil, output.JSON(), nil
}

func (execution *execution) lowestForkFailure() agent.Failure {
	for _, child := range execution.state.ForkWindow {
		if child.Failure != nil && child.Failure.Valid() {
			return *child.Failure
		}
	}
	return agent.Failure{}
}

func (execution *execution) forkStartedChildren() []agent.ProcessID {
	children := make([]agent.ProcessID, 0, len(execution.state.ForkWindow))
	for _, child := range execution.state.ForkWindow {
		if child.ProcessID != nil {
			children = append(children, *child.ProcessID)
		}
	}
	return children
}

func (execution *execution) forkChildKey(branchIndex uint32) (agent.ChildKey, error) {
	branch := execution.stage().fork.branches[branchIndex]
	return workflowChildKey("fork", execution.stage().id, branch.id)
}

func (execution *execution) forkWaitKey() (agent.WaitKey, error) {
	windowStart := execution.state.ForkNext - uint32(len(execution.state.ForkWindow))
	return workflowWaitKey("fork", execution.stage().id, strconv.FormatUint(uint64(windowStart), 10))
}

func (execution *execution) clearFork() {
	execution.state.ForkNext = 0
	execution.state.ForkWindow = nil
	execution.state.ForkOutputs = nil
}
