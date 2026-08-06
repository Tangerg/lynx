package planning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	agent "github.com/Tangerg/lynx/agent2"
)

type execution struct {
	definition *Definition
	state      executionState
}

// Step advances exactly one pure Planning boundary. Observation, dispatcher
// Action I/O, and child Process work are represented as Effects and never run
// inside this method.
func (execution *execution) Step(ctx context.Context, signals []agent.Signal) (agent.Transition, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	if err := execution.state.validate(execution.definition); err != nil {
		return agent.Transition{}, err
	}
	switch execution.state.Phase {
	case phaseReadyObservation:
		if len(signals) != 0 {
			return agent.Transition{}, errors.New("planning: initial observation does not accept Signals")
		}
		return execution.requestObservation(0)
	case phaseAwaitingObservation:
		return execution.acceptObservation(ctx, signals)
	case phaseAwaitingAction:
		return execution.acceptAction(signals)
	case phaseAwaitingChildStart:
		return execution.acceptChildStart(signals)
	case phaseAwaitingChildWaitOpen:
		return execution.acceptChildWaitOpen(signals)
	case phaseWaitingChild:
		return execution.acceptChildCompletion(signals)
	case phaseCompleted:
		return agent.Transition{}, fmt.Errorf("%w: completed execution cannot advance", ErrInvalidExecutionState)
	default:
		return agent.Transition{}, ErrInvalidExecutionState
	}
}

// Snapshot returns the complete, self-sufficient Planning state. It contains
// only Strategy-owned portable values and Framework child identities.
func (execution *execution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.ExecutionState{}, ErrInvalidExecutionState
	}
	if err := execution.state.validate(execution.definition); err != nil {
		return agent.ExecutionState{}, err
	}
	return encodeExecutionState(execution.state)
}

func (execution *execution) requestObservation(consumed uint32) (agent.Transition, error) {
	input, err := execution.state.input()
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := newObservationEffect(input)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseAwaitingObservation
	return agent.Continue(consumed, effect)
}

func (execution *execution) acceptObservation(
	ctx context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	signal, err := oneSignal(signals)
	if err != nil {
		return agent.Transition{}, err
	}
	envelope, err := decodeSignal(signal.Payload())
	if err != nil || envelope.Operation != operationObserve {
		return agent.Transition{}, fmt.Errorf("%w: expected observation Signal", ErrInvalidProtocol)
	}
	consumed := uint32(len(signals))
	if envelope.Observation.Error != "" {
		return execution.fail(
			consumed, agent.FailureKindExternal, "planning.observation.failed", envelope.Observation.Error,
		)
	}
	execution.state.World = *envelope.Observation.WorldState
	if execution.state.PendingConfirm {
		if err := execution.confirmAction(); err != nil {
			return agent.Transition{}, err
		}
	}
	if execution.definition.goal.SatisfiedBy(execution.state.World) {
		return execution.complete(consumed, OutcomeAchieved)
	}
	if uint64(len(execution.state.Attempts)) >= uint64(execution.definition.maxActionAttempts) {
		return execution.complete(consumed, OutcomeStuck)
	}
	if execution.state.PlanningPasses == math.MaxUint32 {
		return execution.fail(
			consumed, agent.FailureKindExecution, "planning.limit.planning_passes",
			"Planning exhausted its representable planning-pass count",
		)
	}
	problem, err := execution.definition.problem(execution.state.World, execution.state.ExcludedActions)
	if err != nil {
		return execution.fail(consumed, agent.FailureKindContract, "planning.problem.invalid", err.Error())
	}
	plan, found, err := execution.definition.planner.Plan(ctx, problem)
	if err != nil {
		return execution.fail(consumed, agent.FailureKindExecution, "planning.planner.failed", err.Error())
	}
	if !found {
		execution.state.PlanningPasses++
		outcome := OutcomeUnreachable
		if len(execution.state.Attempts) > 0 {
			outcome = OutcomeStuck
		}
		return execution.complete(consumed, outcome)
	}
	if err := problem.ValidatePlan(plan); err != nil {
		return execution.fail(consumed, agent.FailureKindContract, "planning.planner.contract", err.Error())
	}
	actions := plan.Actions()
	binding, found := execution.definition.binding(actions[0].Name())
	if !found {
		return execution.fail(
			consumed, agent.FailureKindContract, "planning.planner.contract",
			"Planner selected an Action outside the Planning Definition",
		)
	}
	return execution.startAction(consumed, binding)
}

func (execution *execution) startAction(
	consumed uint32,
	binding ActionBinding,
) (agent.Transition, error) {
	input, err := execution.state.input()
	if err != nil {
		return agent.Transition{}, err
	}
	switch binding.target {
	case bindingTargetDispatcher:
		effect, err := newActionEffect(input, binding, execution.state.World)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.PlanningPasses++
		execution.state.CurrentAction = binding.action.name
		execution.state.Phase = phaseAwaitingAction
		return agent.Continue(consumed, effect)
	case bindingTargetChild:
		childInput := input
		if binding.childInput != nil {
			childInput, err = binding.childInput(input, execution.state.World)
			if err != nil {
				return execution.fail(
					consumed, agent.FailureKindContract, "planning.child.input.failed", err.Error(),
				)
			}
		}
		if !childInput.Valid() {
			return execution.fail(
				consumed, agent.FailureKindContract, "planning.child.input.invalid",
				"Child input function returned an invalid Input",
			)
		}
		key, err := planningChildKey(binding.action.name, uint32(len(execution.state.Attempts)+1))
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.StartChild(binding.childSpec(key, childInput))
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.PlanningPasses++
		execution.state.CurrentAction = binding.action.name
		execution.state.ChildKey = &key
		execution.state.Phase = phaseAwaitingChildStart
		return agent.Continue(consumed, effect)
	default:
		return agent.Transition{}, ErrInvalidAction
	}
}

func (execution *execution) acceptAction(signals []agent.Signal) (agent.Transition, error) {
	signal, err := oneSignal(signals)
	if err != nil {
		return agent.Transition{}, err
	}
	envelope, err := decodeSignal(signal.Payload())
	if err != nil || envelope.Operation != operationAction {
		return agent.Transition{}, fmt.Errorf("%w: expected Action Signal", ErrInvalidProtocol)
	}
	consumed := uint32(len(signals))
	if envelope.Action.Succeeded {
		execution.state.PendingConfirm = true
		return execution.requestObservation(consumed)
	}
	execution.recordFailedAction(envelope.Action.Diagnostic)
	return execution.requestObservation(consumed)
}

func (execution *execution) acceptChildStart(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 {
		return agent.Transition{}, errors.New("planning: child start requires exactly one settlement Signal")
	}
	result, err := agent.ParseChildStartResult(signals[0])
	binding, found := execution.definition.binding(execution.state.CurrentAction)
	if err != nil || !found || binding.target != bindingTargetChild || execution.state.ChildKey == nil ||
		result.Key() != *execution.state.ChildKey || result.DeploymentRef() != binding.child.Deployment {
		return agent.Transition{}, fmt.Errorf("%w: child-start result mismatch", ErrInvalidProtocol)
	}
	consumed := uint32(len(signals))
	if failure, failed := result.Failure(); failed {
		execution.recordFailedAction(failure.Code() + ": " + failure.Message())
		execution.clearChild()
		return execution.requestObservation(consumed)
	}
	childID, started := result.ProcessID()
	if !started {
		return agent.Transition{}, fmt.Errorf("%w: child-start result has no Process", ErrInvalidProtocol)
	}
	waitKey, err := planningChildWaitKey(*execution.state.ChildKey, childID)
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
	return agent.Continue(consumed, effect)
}

func (execution *execution) acceptChildWaitOpen(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 || execution.state.ChildKey == nil || execution.state.ChildProcessID == nil {
		return agent.Transition{}, errors.New("planning: child wait opening requires exactly one settlement Signal")
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	if err != nil {
		return agent.Transition{}, fmt.Errorf("%w: child wait opening: %v", ErrInvalidProtocol, err)
	}
	wantKey, err := planningChildWaitKey(*execution.state.ChildKey, *execution.state.ChildProcessID)
	spec := opened.Spec()
	if err != nil || spec.Key != wantKey || len(spec.Children) != 1 ||
		spec.Children[0] != *execution.state.ChildProcessID || spec.Condition != agent.AllChildren() {
		return agent.Transition{}, fmt.Errorf("%w: child wait opening mismatch", ErrInvalidProtocol)
	}
	waitID := opened.WaitID()
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingChild
	return agent.Wait(uint32(len(signals)), waitID)
}

func (execution *execution) acceptChildCompletion(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 || execution.state.ChildKey == nil || execution.state.ChildProcessID == nil || execution.state.WaitID == nil {
		return agent.Transition{}, errors.New("planning: child completion requires one active child wait Signal")
	}
	completed, err := agent.ParseChildrenCompleted(signals[0])
	if err != nil || completed.WaitID() != *execution.state.WaitID {
		return agent.Transition{}, fmt.Errorf("%w: child completion mismatch", ErrInvalidProtocol)
	}
	wantWaitKey, err := planningChildWaitKey(*execution.state.ChildKey, *execution.state.ChildProcessID)
	if err != nil || completed.Key() != wantWaitKey {
		return agent.Transition{}, fmt.Errorf("%w: child completion wait key mismatch", ErrInvalidProtocol)
	}
	outcomes := completed.Outcomes()
	if len(outcomes) != 1 || outcomes[0].Key() != *execution.state.ChildKey ||
		outcomes[0].Result().ProcessID() != *execution.state.ChildProcessID {
		return agent.Transition{}, fmt.Errorf("%w: child completion outcome mismatch", ErrInvalidProtocol)
	}
	result := outcomes[0].Result()
	execution.clearChild()
	if result.Status() == agent.StatusCompleted {
		execution.state.PendingConfirm = true
	} else {
		execution.recordFailedAction(result.Termination().Reason())
	}
	return execution.requestObservation(uint32(len(signals)))
}

func (execution *execution) confirmAction() error {
	binding, found := execution.definition.binding(execution.state.CurrentAction)
	if !found {
		return ErrInvalidExecutionState
	}
	if execution.state.World.Satisfies(binding.action.effects...) {
		execution.state.Attempts = append(execution.state.Attempts, Attempt{
			Action: execution.state.CurrentAction, Status: AttemptSucceeded,
		})
	} else {
		execution.state.Attempts = append(execution.state.Attempts, Attempt{
			Action: execution.state.CurrentAction, Status: AttemptUnconfirmed,
			Diagnostic: "Reobservation did not establish the Action's predicted effects",
		})
		execution.state.exclude(execution.state.CurrentAction)
	}
	execution.state.CurrentAction = ""
	execution.state.PendingConfirm = false
	return nil
}

func (execution *execution) recordFailedAction(reason string) {
	execution.state.Attempts = append(execution.state.Attempts, Attempt{
		Action: execution.state.CurrentAction, Status: AttemptFailed, Diagnostic: diagnostic(reason),
	})
	execution.state.exclude(execution.state.CurrentAction)
	execution.state.CurrentAction = ""
	execution.state.PendingConfirm = false
}

func (execution *execution) clearChild() {
	execution.state.ChildKey = nil
	execution.state.ChildProcessID = nil
	execution.state.WaitID = nil
}

func (execution *execution) complete(consumed uint32, outcome Outcome) (agent.Transition, error) {
	attempts := slices.Clone(execution.state.Attempts)
	if attempts == nil {
		attempts = []Attempt{}
	}
	output := Output{
		Outcome: outcome, WorldState: execution.state.World,
		Attempts: attempts, PlanningPasses: execution.state.PlanningPasses,
	}
	if err := output.Validate(); err != nil {
		return agent.Transition{}, err
	}
	if outcome == OutcomeAchieved && !execution.definition.goal.SatisfiedBy(output.WorldState) {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	erased, err := agent.EncodeOutput(output)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseCompleted
	execution.state.CurrentAction = ""
	execution.state.PendingConfirm = false
	execution.clearChild()
	execution.state.FinalOutput = &output
	return agent.Complete(consumed, erased)
}

func (execution *execution) fail(
	consumed uint32,
	kind agent.FailureKind,
	code string,
	message string,
) (agent.Transition, error) {
	failure, err := agent.NewFailure(kind, code, diagnostic(message))
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Fail(consumed, failure)
}

func planningChildKey(action string, attempt uint32) (agent.ChildKey, error) {
	return agent.ParseChildKey("planning.action." + planningIdentity(action, attempt))
}

func planningChildWaitKey(childKey agent.ChildKey, childID agent.ProcessID) (agent.WaitKey, error) {
	hash := sha256.New()
	hash.Write([]byte(childKey.String()))
	hash.Write([]byte{0})
	hash.Write([]byte(childID.String()))
	return agent.ParseWaitKey("planning.child." + hex.EncodeToString(hash.Sum(nil)))
}

func planningIdentity(action string, attempt uint32) string {
	hash := sha256.New()
	hash.Write([]byte(action))
	hash.Write([]byte{0})
	hash.Write([]byte(strconv.FormatUint(uint64(attempt), 10)))
	return hex.EncodeToString(hash.Sum(nil))
}
