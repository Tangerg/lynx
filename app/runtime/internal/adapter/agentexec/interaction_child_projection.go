package agentexec

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	corechat "github.com/Tangerg/scope/core/chat"
)

func (i *interactionSession) sendExecutorRequest(
	ctx context.Context,
	event runs.ExecutorEvent,
) error {
	ctx, cancel := i.lifetime.bind(ctx)
	defer cancel()
	select {
	case i.lifetime.events <- event:
		return nil
	case <-i.lifetime.releasing:
		return errors.New("agentexec: execution released before executor request")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// reconcileCompletedDelegateChildren projects terminal children in postorder.
// Event listeners only wake this check; public Process state is authoritative.
func (i *interactionSession) reconcileCompletedDelegateChildren(
	ctx context.Context,
) (bool, error) {
	i.childProjection.lock()
	defer i.childProjection.unlock()
	i.state.mu.Lock()
	calls := make([]*managedDelegateCall, 0, len(i.state.delegateChildren))
	for _, managed := range i.state.delegateChildren {
		calls = append(calls, managed)
	}
	i.state.mu.Unlock()
	slices.SortFunc(calls, func(left, right *managedDelegateCall) int {
		if depth := int(right.parentRelation.Depth()+1) - int(left.parentRelation.Depth()+1); depth != 0 {
			return depth
		}
		if parent := strings.Compare(left.identity.parentID.String(), right.identity.parentID.String()); parent != 0 {
			return parent
		}
		if left.modelCallSequence != right.modelCallSequence {
			return cmp.Compare(left.modelCallSequence, right.modelCallSequence)
		}
		if left.toolCallIndex != right.toolCallIndex {
			return cmp.Compare(left.toolCallIndex, right.toolCallIndex)
		}
		return strings.Compare(left.childProcessID.String(), right.childProcessID.String())
	})
	type delegateBatch struct {
		parentID          agent.ProcessID
		modelCallSequence uint32
	}
	blocked := make(map[delegateBatch]struct{})
	progressed := false
	for _, managed := range calls {
		managed.mu.Lock()
		processID := managed.childProcessID
		done := managed.parentToolFinished
		batch := delegateBatch{
			parentID:          managed.identity.parentID,
			modelCallSequence: managed.modelCallSequence,
		}
		managed.mu.Unlock()
		if done || !processID.Valid() {
			continue
		}
		if _, predecessorPending := blocked[batch]; predecessorPending {
			continue
		}
		process, found := i.engine.Process(processID)
		if !found || !process.Status().Terminal() {
			blocked[batch] = struct{}{}
			continue
		}
		result, err := process.Await(ctx)
		if err != nil {
			return progressed, fmt.Errorf("agentexec: await delegated child %s: %w", processID, err)
		}
		if err := i.projectDelegateResult(ctx, managed, result); err != nil {
			return progressed, err
		}
		progressed = true
	}
	return progressed, nil
}

func (i *interactionSession) projectDelegateResult(
	ctx context.Context,
	managed *managedDelegateCall,
	result agent.Result,
) error {
	committedReply, replyFound := i.committedReplies.lookup(result.ProcessID())
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if result.ProcessID() != managed.childProcessID {
		return errors.New("agentexec: delegated result changed child identity")
	}
	member := runs.ExecutorMember{
		MemberID: result.ProcessID().String(), ParentID: managed.identity.parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	var parentResult string
	var childFailure error
	if result.Status() == agent.StatusCompleted {
		erased, present := result.Output()
		if !present {
			return errors.New("agentexec: completed delegated child has no output")
		}
		if !managed.assistantProjected {
			if !replyFound {
				return errors.New("agentexec: completed delegated child has no committed model reply")
			}
			if !messageRequestsTools(committedReply) {
				if err := i.commitFact(
					ctx, member, runs.AssistantMessageCompleted{Message: committedReply},
				); err != nil {
					return fmt.Errorf("agentexec: commit delegated child answer: %w", err)
				}
			}
			managed.assistantProjected = true
		}
		parentResult = string(erased.JSON())
	} else {
		termination := result.Termination()
		detail := "delegated child ended with " + result.Status().String() +
			" (" + termination.Cause().String() + ")"
		if termination.Reason() != "" {
			detail += ": " + termination.Reason()
		}
		childFailure = errors.New(detail)
		parentResult = "error: delegated worker " + detail
	}
	if !managed.segmentProjected {
		if err := i.sendExecutorRequest(ctx, runs.ExecutorEvent{
			Member: member, Payload: i.segmentEnd(result),
		}); err != nil {
			return fmt.Errorf("agentexec: publish delegated child terminal: %w", err)
		}
		managed.segmentProjected = true
		i.committedReplies.forget(result.ProcessID())
	}
	if err := i.finishDelegateTool(ctx, managed, parentResult, childFailure); err != nil {
		return err
	}
	return nil
}

func messageRequestsTools(message corechat.Message) bool {
	for _, part := range message.Parts {
		if part.Kind == corechat.PartToolCall {
			return true
		}
	}
	return false
}

func (i *interactionSession) finishDelegateTool(
	ctx context.Context,
	managed *managedDelegateCall,
	output string,
	cause error,
) error {
	if managed.parentToolFinished {
		return nil
	}
	if !managed.toolStarted {
		return errors.New("agentexec: cannot finish a Delegate Tool before its start")
	}
	result := tool.StringResult(output)
	if parsed, err := tool.ParseResult([]byte(output)); err == nil {
		result = parsed
	}
	fact := runs.ToolCallFinished{
		CallID: managed.callID, Arguments: managed.arguments.Canonical(), Result: &result,
	}
	if cause != nil {
		fact.Failure = &tool.Failure{
			Kind:   tool.FailureExecution,
			Detail: executorDiagnostic(cause),
		}
	}
	if err := i.commitFact(ctx, i.executorMember(managed.parentRelation), fact); err != nil {
		return fmt.Errorf("agentexec: commit Delegate Tool result: %w", err)
	}
	managed.parentToolFinished = true
	return nil
}
