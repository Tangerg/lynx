package agentexec

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// delegateCallIdentity mirrors Agent2's documented parent-scoped ChildKey
// identity without copying any Framework tree wire into Runtime state.
type delegateCallIdentity struct {
	parentID agent.ProcessID
	childKey agent.ChildKey
}

type managedDelegateCall struct {
	mu sync.Mutex

	identity           delegateCallIdentity
	parentRelation     agent.ProcessRelation
	target             agent.DeploymentRef
	call               corechat.ToolCall
	input              delegation.Input
	arguments          tool.Arguments
	modelCallSequence  uint32
	toolCallIndex      uint32
	callID             string
	admission          agent.ProcessAdmission
	binding            runs.ChildRunBinding
	childProcessID     agent.ProcessID
	toolStarted        bool
	parentToolFinished bool
	assistantProjected bool
	segmentProjected   bool
}

func (session *interactionSession) installDeployments(deployments *interactionDeploymentSet) error {
	if deployments == nil || !deployments.root.Valid() {
		return errors.New("agentexec: install invalid native Interaction deployments")
	}
	session.mu.Lock()
	session.deployments = deployments
	session.deployment = deployments.root
	session.mu.Unlock()
	return nil
}

// registerDelegateCalls records only model calls whose exact Deployment has a
// managed Delegate binding. Invalid delegate arguments never reach admission
// and are therefore intentionally absent from this lifecycle correlation.
func (session *interactionSession) registerDelegateCalls(
	invocation interaction.ModelInvocation,
	message *corechat.Message,
) error {
	if !invocation.Valid() || message == nil {
		return errors.New("agentexec: cannot register unattributed Delegate calls")
	}
	session.mu.Lock()
	deployments := session.deployments
	session.mu.Unlock()
	if deployments == nil {
		return errors.New("agentexec: native Interaction deployments are unavailable")
	}
	toolCallIndex := uint32(0)
	for _, part := range message.Parts {
		if part.Kind != corechat.PartToolCall || part.ToolCall == nil {
			continue
		}
		call := *part.ToolCall
		target, managed := deployments.delegateTarget(invocation.DeploymentRef(), call.Name)
		if !managed {
			toolCallIndex++
			continue
		}
		input, arguments, err := decodeDelegateCall(call)
		if err != nil {
			// Agent2 applies the same Descriptor contract before creating a child
			// Effect and returns an ordinary Tool error for malformed input.
			toolCallIndex++
			continue
		}
		childKey, err := interaction.DelegateChildKey(invocation.ModelCallSequence(), call)
		if err != nil {
			return err
		}
		identity := delegateCallIdentity{
			parentID: invocation.Relation().ProcessID(), childKey: childKey,
		}
		managedCall := &managedDelegateCall{
			identity: identity, parentRelation: invocation.Relation(), target: target,
			call: call, input: input, arguments: arguments,
			modelCallSequence: invocation.ModelCallSequence(), toolCallIndex: toolCallIndex,
			callID: delegatedToolCallID(
				invocation.Relation(), invocation.ModelCallSequence(), toolCallIndex, call,
			),
		}
		session.mu.Lock()
		if prior := session.delegateCalls[identity]; prior != nil {
			session.mu.Unlock()
			return fmt.Errorf(
				"agentexec: Delegate child %q was registered more than once for parent %s",
				childKey, invocation.Relation().ProcessID(),
			)
		}
		session.delegateCalls[identity] = managedCall
		session.mu.Unlock()
		toolCallIndex++
	}
	return nil
}

func decodeDelegateCall(call corechat.ToolCall) (delegation.Input, tool.Arguments, error) {
	rawArguments := strings.TrimSpace(call.Arguments)
	if rawArguments == "" {
		rawArguments = "{}"
	}
	erased, err := agent.ParseInput([]byte(rawArguments))
	if err != nil {
		return delegation.Input{}, tool.Arguments{}, err
	}
	input, err := agent.DecodeInput[delegation.Input](erased)
	if err != nil {
		return delegation.Input{}, tool.Arguments{}, err
	}
	if err := input.Validate(); err != nil {
		return delegation.Input{}, tool.Arguments{}, err
	}
	arguments, err := tool.ParseArguments(string(erased.JSON()))
	if err != nil {
		return delegation.Input{}, tool.Arguments{}, err
	}
	return input, arguments, nil
}

func (session *interactionSession) executorMember(
	relation agent.ProcessRelation,
) runs.ExecutorMember {
	member := basicExecutorMember(relation)
	if relation.IsRoot() {
		return member
	}
	session.mu.Lock()
	managed := session.delegateChildren[relation.ProcessID()]
	session.mu.Unlock()
	if managed == nil {
		return member
	}
	managed.mu.Lock()
	member.SpawnCallID = managed.call.ID
	managed.mu.Unlock()
	return member
}

func (session *interactionSession) executorMemberByProcessID(
	processID agent.ProcessID,
) (runs.ExecutorMember, bool) {
	if !processID.Valid() {
		return runs.ExecutorMember{}, false
	}
	session.mu.Lock()
	root := session.process
	managed := session.delegateChildren[processID]
	session.mu.Unlock()
	if root != nil && root.ID() == processID {
		return basicExecutorMember(root.Relation()), true
	}
	if managed == nil {
		return runs.ExecutorMember{}, false
	}
	managed.mu.Lock()
	member := runs.ExecutorMember{
		MemberID: processID.String(), ParentID: managed.identity.parentID.String(),
		SpawnCallID: managed.call.ID,
	}
	managed.mu.Unlock()
	return member, true
}
