package interaction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

func (execution *execution) startDelegateSegment(
	consumed uint32,
	calls []chat.ToolCall,
) (agent.Transition, bool, error) {
	start := execution.state.NextCall
	end := start
	for end < uint32(len(calls)) {
		if _, delegated := execution.definition.delegate(calls[end].Name); !delegated {
			break
		}
		end++
	}
	segment := delegateSegmentState{Invocations: make([]delegateInvocationState, end-start)}
	effects := make([]agent.Effect, 0, len(segment.Invocations))
	for offset := range segment.Invocations {
		call := calls[start+uint32(offset)]
		delegate, _ := execution.definition.delegate(call.Name)
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		input, err := agent.ParseInput([]byte(arguments))
		if err != nil {
			result := delegateErrorResult(call, "arguments are not valid JSON: "+err.Error())
			segment.Invocations[offset].Result = &result
			continue
		}
		if err := delegate.validateInput(input); err != nil {
			result := delegateErrorResult(call, "arguments violate the delegated worker input contract: "+err.Error())
			segment.Invocations[offset].Result = &result
			continue
		}
		key, err := delegateChildKey(execution.state.ModelCalls, call)
		if err != nil {
			return agent.Transition{}, false, err
		}
		effect, err := agent.StartChild(agent.ChildSpec{
			Key: key, Deployment: delegate.deployment, Input: input,
			Budget: delegate.budget, Capabilities: delegate.capabilities,
		})
		if err != nil {
			return agent.Transition{}, false, err
		}
		segment.Invocations[offset].Key = &key
		effects = append(effects, effect)
	}
	execution.state.ActiveCallEnd = end
	execution.state.DelegateSegment = &segment
	if len(effects) == 0 {
		results, err := delegateSegmentResults(segment)
		if err != nil {
			return agent.Transition{}, false, err
		}
		execution.state.SettledResults = append(execution.state.SettledResults, results...)
		execution.state.NextCall = end
		execution.state.ActiveCallEnd = 0
		execution.state.DelegateSegment = nil
		return agent.Transition{}, false, nil
	}
	execution.state.Phase = phaseAwaitingDelegateStarts
	transition, err := agent.Continue(consumed, effects...)
	return transition, true, err
}

func (execution *execution) acceptDelegateStarts(signals []agent.Signal) (agent.Transition, error) {
	starts, steering, consumed, err := collectChildStarts(signals)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.addSteering(steering)
	calls, _, err := execution.activeCallSegment()
	if err != nil || execution.state.DelegateSegment == nil {
		return agent.Transition{}, ErrInvalidState
	}
	next := 0
	for index := range execution.state.DelegateSegment.Invocations {
		invocation := &execution.state.DelegateSegment.Invocations[index]
		if invocation.Key == nil || invocation.Result != nil {
			continue
		}
		if next >= len(starts) {
			return agent.Transition{}, fmt.Errorf("%w: missing Delegate child-start result", ErrInvalidState)
		}
		start := starts[next]
		next++
		delegate, _ := execution.definition.delegate(calls[index].Name)
		if start.Key() != *invocation.Key || start.DeploymentRef() != delegate.deployment {
			return agent.Transition{}, fmt.Errorf("%w: Delegate child-start result mismatch", ErrInvalidState)
		}
		if failure, failed := start.Failure(); failed {
			result := delegateErrorResult(
				calls[index], "child start failed: "+failure.Code()+": "+failure.Message(),
			)
			invocation.Result = &result
			continue
		}
		processID, started := start.ProcessID()
		if !started {
			return agent.Transition{}, fmt.Errorf("%w: Delegate child start has no outcome", ErrInvalidState)
		}
		invocation.ProcessID = &processID
	}
	if next != len(starts) {
		return agent.Transition{}, fmt.Errorf("%w: unexpected Delegate child-start result", ErrInvalidState)
	}
	children := execution.delegateChildren()
	if len(children) == 0 {
		results, err := delegateSegmentResults(*execution.state.DelegateSegment)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.SettledResults = append(execution.state.SettledResults, results...)
		execution.state.NextCall = execution.state.ActiveCallEnd
		execution.state.ActiveCallEnd = 0
		execution.state.DelegateSegment = nil
		return execution.advanceToolCallBatch(consumed)
	}
	waitKey, err := delegateWaitKey(execution.state.ModelCalls, *execution.state.DelegateSegment)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: waitKey, Children: children, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseAwaitingDelegateWaitID
	return agent.Continue(consumed, effect)
}

func (execution *execution) acceptDelegateWaitID(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, fmt.Errorf("%w: Delegate wait-opened Signal is missing", ErrInvalidState)
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	if err != nil {
		return agent.Transition{}, fmt.Errorf("%w: invalid Delegate wait-opened Signal", ErrInvalidState)
	}
	want, err := execution.delegateWaitSpec()
	if err != nil {
		return agent.Transition{}, err
	}
	got := opened.Spec()
	if got.Key != want.Key || got.Condition != want.Condition || !sameProcessIDs(got.Children, want.Children) {
		return agent.Transition{}, fmt.Errorf("%w: Delegate child-wait opening mismatch", ErrInvalidState)
	}
	waitID := opened.WaitID()
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingDelegates
	return agent.Wait(1, waitID)
}

func (execution *execution) acceptDelegates(signals []agent.Signal) (agent.Transition, error) {
	completed, steering, consumed, err := collectChildrenCompleted(signals)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.addSteering(steering)
	if execution.state.WaitID == nil || completed.WaitID() != *execution.state.WaitID {
		return agent.Transition{}, fmt.Errorf("%w: Delegate child completion addressed the wrong wait", ErrInvalidState)
	}
	want, err := execution.delegateWaitSpec()
	if err != nil || completed.Key() != want.Key {
		return agent.Transition{}, fmt.Errorf("%w: Delegate child completion wait mismatch", ErrInvalidState)
	}
	calls, _, err := execution.activeCallSegment()
	if err != nil {
		return agent.Transition{}, err
	}
	outcomes := completed.Outcomes()
	next := 0
	results := make([]chat.ToolResult, len(calls))
	for index, invocation := range execution.state.DelegateSegment.Invocations {
		if invocation.Result != nil {
			results[index] = *invocation.Result
			continue
		}
		if invocation.Key == nil || invocation.ProcessID == nil || next >= len(outcomes) {
			return agent.Transition{}, fmt.Errorf("%w: missing Delegate child outcome", ErrInvalidState)
		}
		outcome := outcomes[next]
		next++
		if outcome.Key() != *invocation.Key || outcome.Result().ProcessID() != *invocation.ProcessID {
			return agent.Transition{}, fmt.Errorf("%w: Delegate child outcome mismatch", ErrInvalidState)
		}
		result := outcome.Result()
		if result.Status() != agent.StatusCompleted {
			termination := result.Termination()
			diagnostic := "child ended with " + result.Status().String() +
				" (" + termination.Cause().String() + ")"
			if termination.Reason() != "" {
				diagnostic += ": " + termination.Reason()
			}
			results[index] = delegateErrorResult(calls[index], diagnostic)
			continue
		}
		output, present := result.Output()
		delegate, found := execution.definition.delegate(calls[index].Name)
		if !present || !found || delegate.outputSchema.ValidateOutput(output) != nil {
			return agent.Transition{}, fmt.Errorf("%w: Delegate child output violates its frozen contract", ErrInvalidState)
		}
		results[index] = chat.ToolResult{
			ID: calls[index].ID, Name: calls[index].Name, Result: string(output.JSON()),
		}
	}
	if next != len(outcomes) {
		return agent.Transition{}, fmt.Errorf("%w: unexpected Delegate child outcome", ErrInvalidState)
	}
	execution.state.SettledResults = append(execution.state.SettledResults, results...)
	execution.state.NextCall = execution.state.ActiveCallEnd
	execution.state.ActiveCallEnd = 0
	execution.state.DelegateSegment = nil
	execution.state.WaitID = nil
	return execution.advanceToolCallBatch(consumed)
}

func (execution *execution) delegateChildren() []agent.ProcessID {
	if execution.state.DelegateSegment == nil {
		return nil
	}
	children := make([]agent.ProcessID, 0, len(execution.state.DelegateSegment.Invocations))
	for _, invocation := range execution.state.DelegateSegment.Invocations {
		if invocation.ProcessID != nil {
			children = append(children, *invocation.ProcessID)
		}
	}
	return children
}

func (execution *execution) delegateWaitSpec() (agent.ChildWaitSpec, error) {
	if execution.state.DelegateSegment == nil {
		return agent.ChildWaitSpec{}, ErrInvalidState
	}
	key, err := delegateWaitKey(execution.state.ModelCalls, *execution.state.DelegateSegment)
	if err != nil {
		return agent.ChildWaitSpec{}, err
	}
	spec := agent.ChildWaitSpec{
		Key: key, Children: execution.delegateChildren(), Condition: agent.AllChildren(),
	}
	if !spec.Valid() {
		return agent.ChildWaitSpec{}, ErrInvalidState
	}
	return spec, nil
}

func collectChildStarts(signals []agent.Signal) ([]agent.ChildStartResult, []chat.Message, uint32, error) {
	starts := make([]agent.ChildStartResult, 0, len(signals))
	steering := make([]chat.Message, 0)
	for _, signal := range signals {
		if envelope, err := decodeSignal(signal.Payload()); err == nil {
			if envelope.Operation != operationSteer {
				return nil, nil, 0, fmt.Errorf("%w: unexpected Interaction Signal while awaiting Delegate starts", ErrInvalidState)
			}
			steering = append(steering, cloneMessages(envelope.Steer.Messages)...)
			continue
		}
		start, err := agent.ParseChildStartResult(signal)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("%w: invalid Delegate child-start Signal", ErrInvalidState)
		}
		starts = append(starts, start)
	}
	if len(starts) == 0 {
		return nil, nil, 0, fmt.Errorf("%w: Delegate child-start Signal is missing", ErrInvalidState)
	}
	return starts, steering, uint32(len(signals)), nil
}

func collectChildrenCompleted(signals []agent.Signal) (agent.ChildrenCompleted, []chat.Message, uint32, error) {
	var completed agent.ChildrenCompleted
	var found bool
	var steering []chat.Message
	for _, signal := range signals {
		if envelope, err := decodeSignal(signal.Payload()); err == nil {
			if envelope.Operation != operationSteer {
				return agent.ChildrenCompleted{}, nil, 0, fmt.Errorf("%w: unexpected Interaction Signal while awaiting Delegates", ErrInvalidState)
			}
			steering = append(steering, cloneMessages(envelope.Steer.Messages)...)
			continue
		}
		value, err := agent.ParseChildrenCompleted(signal)
		if err != nil || found {
			return agent.ChildrenCompleted{}, nil, 0, fmt.Errorf("%w: invalid or duplicate Delegate completion Signal", ErrInvalidState)
		}
		completed, found = value, true
	}
	if !found {
		return agent.ChildrenCompleted{}, nil, 0, fmt.Errorf("%w: Delegate completion Signal is missing", ErrInvalidState)
	}
	return completed, steering, uint32(len(signals)), nil
}

func delegateSegmentResults(segment delegateSegmentState) ([]chat.ToolResult, error) {
	results := make([]chat.ToolResult, len(segment.Invocations))
	for index, invocation := range segment.Invocations {
		if invocation.Result == nil {
			return nil, fmt.Errorf("%w: Delegate call %d is not settled", ErrInvalidState, index)
		}
		results[index] = *invocation.Result
	}
	return results, nil
}

func delegateErrorResult(call chat.ToolCall, diagnostic string) chat.ToolResult {
	return chat.ToolResult{
		ID: call.ID, Name: call.Name,
		Result: "error: delegated worker " + boundedDiagnostic(diagnostic), IsError: true,
	}
}

func delegateChildKey(modelCalls uint32, call chat.ToolCall) (agent.ChildKey, error) {
	hash := sha256.New()
	hash.Write([]byte(strconv.FormatUint(uint64(modelCalls), 10)))
	hash.Write([]byte{0})
	hash.Write([]byte(call.ID))
	hash.Write([]byte{0})
	hash.Write([]byte(call.Name))
	return agent.ParseChildKey("interaction.delegate.child." + hex.EncodeToString(hash.Sum(nil)))
}

func delegateWaitKey(modelCalls uint32, segment delegateSegmentState) (agent.WaitKey, error) {
	hash := sha256.New()
	hash.Write([]byte(strconv.FormatUint(uint64(modelCalls), 10)))
	for _, invocation := range segment.Invocations {
		if invocation.ProcessID == nil || invocation.Key == nil {
			continue
		}
		hash.Write([]byte{0})
		hash.Write([]byte(invocation.Key.String()))
		hash.Write([]byte{0})
		hash.Write([]byte(invocation.ProcessID.String()))
	}
	return agent.ParseWaitKey("interaction.delegate.wait." + hex.EncodeToString(hash.Sum(nil)))
}

func sameProcessIDs(left, right []agent.ProcessID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
