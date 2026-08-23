package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

type processBinding struct {
	runID, segmentID, parentRunID, rootRunID string
	depth                                   uint32
}

type delegateIdentity struct {
	parentID agent.ProcessID
	childKey agent.ChildKey
}

type delegateIntent struct {
	target      agent.DeploymentRef
	callID      string
	task        DelegateTask
	requestedAt time.Time
}

type pendingDelegate struct {
	admission agent.ProcessAdmission
	binding   DelegateBinding
}

// delegationBridge is the only adapter object allowed to correlate Framework
// tree identities with product Runs. It stores no protocol state and delegates
// every durable decision to the Run application service.
type delegationBridge struct {
	mu          sync.RWMutex
	coordinator DelegationCoordinator
	root        processBinding
	rootRef     agent.DeploymentRef
	targets     map[agent.DeploymentRef]agent.DeploymentRef
	bindings    map[agent.ProcessID]processBinding
	intents     map[delegateIdentity]delegateIntent
	conflicts   map[delegateIdentity]bool
	pending     map[agent.ProcessID]pendingDelegate
}

func newDelegationBridge(rootRunID, rootSegmentID string, coordinator DelegationCoordinator) (*delegationBridge, error) {
	if rootRunID == "" || rootSegmentID == "" || coordinator == nil {
		return nil, errors.New("agentexec: delegated execution requires root identity and coordinator")
	}
	return &delegationBridge{
		coordinator: coordinator,
		root: processBinding{runID: rootRunID, segmentID: rootSegmentID, rootRunID: rootRunID},
		targets: make(map[agent.DeploymentRef]agent.DeploymentRef),
		bindings: make(map[agent.ProcessID]processBinding), intents: make(map[delegateIdentity]delegateIntent),
		conflicts: make(map[delegateIdentity]bool), pending: make(map[agent.ProcessID]pendingDelegate),
	}, nil
}

func (bridge *delegationBridge) installFamily(root agent.DeploymentRef, targets map[agent.DeploymentRef]agent.DeploymentRef) {
	bridge.mu.Lock()
	bridge.rootRef = root
	for parent, child := range targets {
		bridge.targets[parent] = child
	}
	bridge.mu.Unlock()
}

func (bridge *delegationBridge) binding(relation agent.ProcessRelation) (processBinding, bool) {
	if bridge == nil || !relation.Valid() {
		return processBinding{}, false
	}
	bridge.mu.RLock()
	binding, found := bridge.bindings[relation.ProcessID()]
	bridge.mu.RUnlock()
	return binding, found
}

func (bridge *delegationBridge) bindingProcess(processID agent.ProcessID) (processBinding, bool) {
	if bridge == nil || !processID.Valid() {
		return processBinding{}, false
	}
	bridge.mu.RLock()
	binding, found := bridge.bindings[processID]
	bridge.mu.RUnlock()
	return binding, found
}

func (bridge *delegationBridge) processForRun(runID string) (agent.ProcessID, bool) {
	if bridge == nil || runID == "" {
		return agent.ProcessID{}, false
	}
	bridge.mu.RLock()
	defer bridge.mu.RUnlock()
	for processID, binding := range bridge.bindings {
		if binding.runID == runID {
			return processID, true
		}
	}
	return agent.ProcessID{}, false
}

func (bridge *delegationBridge) subtree(runID string) map[agent.ProcessID]bool {
	selected := make(map[agent.ProcessID]bool)
	if bridge == nil || runID == "" {
		return selected
	}
	bridge.mu.RLock()
	defer bridge.mu.RUnlock()
	for processID, binding := range bridge.bindings {
		current := binding
		for current.runID != "" {
			if current.runID == runID {
				selected[processID] = true
				break
			}
			if current.parentRunID == "" {
				break
			}
			parentFound := false
			for _, candidate := range bridge.bindings {
				if candidate.runID == current.parentRunID {
					current = candidate
					parentFound = true
					break
				}
			}
			if !parentFound {
				break
			}
		}
	}
	return selected
}

func (bridge *delegationBridge) restoreBindings(
	tree agent.TreeSnapshot,
	members []TreeResumeMember,
) error {
	if bridge == nil || !tree.Valid() || len(members) == 0 {
		return errors.New("agentexec: invalid restored delegation bindings")
	}
	snapshots := make(map[agent.ProcessID]agent.Snapshot, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		snapshots[snapshot.ProcessID()] = snapshot
	}
	bound := make(map[agent.ProcessID]processBinding, len(members))
	runs := make(map[string]agent.ProcessID, len(members))
	for _, member := range members {
		if member.RunID == "" || member.SegmentID == "" || member.RootRunID != bridge.root.rootRunID ||
			member.Depth == 0 && member.RunID != bridge.root.runID || runs[member.RunID].Valid() {
			return errors.New("agentexec: restored member identity is invalid")
		}
		processID := tree.RootID()
		if member.RunID == bridge.root.runID {
			if member.MemberID != "" || member.ParentRunID != "" || member.Depth != 0 ||
				member.SegmentID != bridge.root.segmentID {
				return errors.New("agentexec: restored root binding changed identity")
			}
		} else {
			var err error
			processID, err = agent.ParseProcessID(member.MemberID)
			if err != nil || member.ParentRunID == "" || member.Depth == 0 {
				return errors.New("agentexec: restored child binding is invalid")
			}
		}
		snapshot, found := snapshots[processID]
		if !found || snapshot.Status().Terminal() || snapshot.Relation().Depth() != member.Depth {
			return errors.New("agentexec: restored member differs from tree checkpoint")
		}
		binding := processBinding{
			runID: member.RunID, segmentID: member.SegmentID,
			parentRunID: member.ParentRunID, rootRunID: member.RootRunID, depth: member.Depth,
		}
		if prior, duplicate := bound[processID]; duplicate && prior != binding {
			return errors.New("agentexec: restored Process has conflicting Run bindings")
		}
		bound[processID] = binding
		runs[member.RunID] = processID
	}
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Status().Terminal() {
			continue
		}
		binding, found := bound[snapshot.ProcessID()]
		if !found {
			return fmt.Errorf("agentexec: checkpointed Process %s has no resumed Run", snapshot.ProcessID())
		}
		if parentID, child := snapshot.Relation().ParentID(); child {
			parent, found := bound[parentID]
			if !found || binding.parentRunID != parent.runID {
				return errors.New("agentexec: restored Run parent differs from Process relation")
			}
		}
	}
	bridge.mu.Lock()
	for processID, binding := range bound {
		bridge.bindings[processID] = binding
	}
	bridge.mu.Unlock()
	return nil
}

func (bridge *delegationBridge) register(invocation interaction.ModelInvocation, response *chat.Response, occurredAt time.Time) {
	if bridge == nil || !invocation.Valid() || response == nil {
		return
	}
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return
	}
	bridge.mu.RLock()
	target, delegated := bridge.targets[invocation.DeploymentRef()]
	bridge.mu.RUnlock()
	if !delegated {
		return
	}
	for _, part := range choice.Message.Parts {
		if part.Kind != chat.PartToolCall || part.ToolCall == nil || part.ToolCall.Name != DelegateToolName {
			continue
		}
		call := *part.ToolCall
		task, err := decodeDelegateTask(call.Arguments)
		if err != nil {
			continue
		}
		key, err := interaction.DelegateChildKey(invocation.ModelCallSequence(), call)
		if err != nil {
			continue
		}
		identity := delegateIdentity{parentID: invocation.Relation().ProcessID(), childKey: key}
		intent := delegateIntent{target: target, callID: call.ID, task: task, requestedAt: occurredAt.UTC()}
		bridge.mu.Lock()
		if prior, exists := bridge.intents[identity]; exists && !sameDelegateIntent(prior, intent) {
			bridge.conflicts[identity] = true
		} else {
			bridge.intents[identity] = intent
		}
		bridge.mu.Unlock()
	}
}

func decodeDelegateTask(arguments string) (DelegateTask, error) {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		arguments = "{}"
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var task DelegateTask
	if err := decoder.Decode(&task); err != nil {
		return DelegateTask{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return DelegateTask{}, errors.New("agentexec: delegated arguments contain trailing data")
		}
		return DelegateTask{}, fmt.Errorf("agentexec: decode delegated arguments: %w", err)
	}
	return task, task.Validate()
}

func sameDelegateIntent(left, right delegateIntent) bool {
	return left.target == right.target && left.callID == right.callID && left.task == right.task
}

func (bridge *delegationBridge) Admit(ctx context.Context, admission agent.ProcessAdmission) error {
	if bridge == nil || !admission.Valid() {
		return errors.New("agentexec: invalid Process admission")
	}
	relation := admission.Relation()
	if relation.IsRoot() {
		bridge.mu.Lock()
		defer bridge.mu.Unlock()
		if admission.DeploymentRef() != bridge.rootRef {
			return errors.New("agentexec: root admission changed deployment")
		}
		if prior, exists := bridge.bindings[relation.ProcessID()]; exists && prior != bridge.root {
			return errors.New("agentexec: root admission changed identity")
		}
		bridge.bindings[relation.ProcessID()] = bridge.root
		return nil
	}
	parentID, _ := relation.ParentID()
	childKey, _ := relation.ChildKey()
	identity := delegateIdentity{parentID: parentID, childKey: childKey}
	bridge.mu.RLock()
	intent, found := bridge.intents[identity]
	parent, parentFound := bridge.bindings[parentID]
	conflicted := bridge.conflicts[identity]
	prior, repeated := bridge.pending[relation.ProcessID()]
	bridge.mu.RUnlock()
	if conflicted || !found || !parentFound || intent.target != admission.DeploymentRef() {
		return errors.New("agentexec: child admission has no exact Delegate intent")
	}
	if repeated {
		if sameAdmission(prior.admission, admission) {
			return nil
		}
		return errors.New("agentexec: repeated child admission changed identity")
	}
	binding, err := bridge.coordinator.ReserveDelegate(ctx, DelegateRequest{
		MemberID: relation.ProcessID().String(), ParentMemberID: parentID.String(), ChildKey: childKey.String(),
		ParentRunID: parent.runID, ParentSegmentID: parent.segmentID, CallID: intent.callID, Task: intent.task,
		RequestedAt: intent.requestedAt, StartedAt: admission.StartedAt(),
	})
	if err != nil {
		return fmt.Errorf("agentexec: reserve child Run: %w", err)
	}
	if !binding.Valid() || binding.ParentRunID != parent.runID || binding.RootRunID != parent.rootRunID {
		return errors.New("agentexec: child Run reservation changed lineage")
	}
	bridge.mu.Lock()
	bridge.pending[relation.ProcessID()] = pendingDelegate{admission: admission, binding: binding}
	bridge.mu.Unlock()
	return nil
}

func (bridge *delegationBridge) AcknowledgeProcessStartOutcome(
	ctx context.Context,
	outcome agent.ProcessStartOutcome,
) error {
	if bridge == nil || !outcome.Valid() {
		return errors.New("agentexec: invalid Process start outcome")
	}
	relation := outcome.Admission().Relation()
	if relation.IsRoot() {
		if outcome.Status() != agent.ProcessStartOutcomeStatusStarted {
			return errors.New("agentexec: accepted root Process aborted")
		}
		return nil
	}
	bridge.mu.RLock()
	pending, found := bridge.pending[relation.ProcessID()]
	bridge.mu.RUnlock()
	if !found || !sameAdmission(pending.admission, outcome.Admission()) {
		return errors.New("agentexec: child start outcome has no admission")
	}
	command := DelegateStartOutcome{MemberID: relation.ProcessID().String()}
	if outcome.Status() == agent.ProcessStartOutcomeStatusStarted {
		command.Started = true
	} else {
		failure, _ := outcome.Failure()
		command.Failure = failure.Code() + ": " + failure.Message()
	}
	binding, err := bridge.coordinator.ConcludeDelegateStart(ctx, command)
	if err != nil {
		return fmt.Errorf("agentexec: conclude child start: %w", err)
	}
	if binding != pending.binding {
		return errors.New("agentexec: child start conclusion changed product identity")
	}
	bridge.mu.Lock()
	if command.Started {
		bridge.bindings[relation.ProcessID()] = processBinding{
			runID: binding.RunID, segmentID: binding.SegmentID,
			parentRunID: binding.ParentRunID, rootRunID: binding.RootRunID, depth: relation.Depth(),
		}
	}
	delete(bridge.pending, relation.ProcessID())
	bridge.mu.Unlock()
	return nil
}

type boundChildProcess struct {
	processID agent.ProcessID
	binding   processBinding
}

func (bridge *delegationBridge) children() []boundChildProcess {
	if bridge == nil {
		return nil
	}
	bridge.mu.RLock()
	children := make([]boundChildProcess, 0, len(bridge.bindings))
	for processID, binding := range bridge.bindings {
		if binding.parentRunID == "" {
			continue
		}
		children = append(children, boundChildProcess{processID: processID, binding: binding})
	}
	bridge.mu.RUnlock()
	sort.Slice(children, func(left, right int) bool {
		if children[left].binding.depth != children[right].binding.depth {
			return children[left].binding.depth > children[right].binding.depth
		}
		return children[left].binding.runID < children[right].binding.runID
	})
	return children
}

func sameAdmission(left, right agent.ProcessAdmission) bool {
	return left.Valid() && right.Valid() && left.Relation() == right.Relation() &&
		left.DeploymentRef() == right.DeploymentRef() && left.StartedAt().Equal(right.StartedAt())
}

var _ agent.ProcessAdmitter = (*delegationBridge)(nil)
var _ agent.ProcessStartOutcomeAcknowledger = (*delegationBridge)(nil)
