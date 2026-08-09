package agentexec

import (
	"errors"
	"fmt"
	"math"
	"slices"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func (session *interactionSession) initializeRestoredContinuation(
	root *agent.Process,
	continuation runs.WaitingContinuation,
	checkpoint interactionCheckpointState,
	boundary interactionBoundary,
) error {
	if boundary != interactionBoundaryWaiting && boundary != interactionBoundaryContinuationStaged {
		return errors.New("invalid restored Interaction boundary")
	}
	if root == nil || root.ID() != checkpoint.tree.RootID() ||
		!isInteractionWaitingBoundary(root.Status()) {
		return fmt.Errorf("%w: restored Interaction root is not at a waiting boundary", runs.ErrExecutorStateLost)
	}
	snapshots := make(map[agent.ProcessID]agent.Snapshot, len(checkpoint.tree.ProcessSnapshots()))
	for _, snapshot := range checkpoint.tree.ProcessSnapshots() {
		snapshots[snapshot.ProcessID()] = snapshot
	}
	members, err := restoredWaitingMembers(continuation, snapshots, root.ID())
	if err != nil {
		return err
	}
	usageByProcess, carriedUsage, err := restoreInteractionAccounting(
		continuation.Checkpoint.Usage, checkpoint, members,
	)
	if err != nil {
		return fmt.Errorf("%w: restore Interaction accounting: %w", runs.ErrExecutorStateLost, err)
	}
	delegateCalls, delegateChildren, err := session.restoreDelegateCalls(snapshots, members)
	if err != nil {
		return fmt.Errorf("%w: restore Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.begun || session.finished || session.process != nil {
		return runs.ErrExecutionClaimed
	}
	session.process = root
	session.admittedProcessID = root.ID()
	session.begun = true
	session.boundary = boundary
	session.waitingCheckpoint = continuation.Checkpoint.Clone()
	session.usageByProcess = usageByProcess
	session.carriedUsage = carriedUsage
	session.delegateCalls = delegateCalls
	session.delegateChildren = delegateChildren
	return nil
}

func restoredWaitingMembers(
	continuation runs.WaitingContinuation,
	snapshots map[agent.ProcessID]agent.Snapshot,
	rootID agent.ProcessID,
) (map[agent.ProcessID]runs.WaitingMember, error) {
	members := make(map[agent.ProcessID]runs.WaitingMember, len(continuation.Members))
	runByProcess := make(map[agent.ProcessID]string, len(continuation.Members))
	for _, member := range continuation.Members {
		processID, err := agent.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, fmt.Errorf("waiting member %q: %w", member.MemberID, err)
		}
		snapshot, found := snapshots[processID]
		if !found || snapshot.Status().Terminal() {
			return nil, fmt.Errorf("waiting member %s has no active Process", processID)
		}
		members[processID] = member
		runByProcess[processID] = member.RunID
	}
	rootMember, found := members[rootID]
	if !found || rootMember.RunID != continuation.RootRunID || rootMember.ParentRunID != "" {
		return nil, errors.New("restored root Process differs from the product root member")
	}
	for processID, member := range members {
		relation := snapshots[processID].Relation()
		if processID == rootID {
			if !relation.IsRoot() {
				return nil, errors.New("restored product root has a child Process relation")
			}
			continue
		}
		parentID, child := relation.ParentID()
		parentRunID, parentSurvives := runByProcess[parentID]
		if !child || !parentSurvives || parentRunID != member.ParentRunID {
			return nil, fmt.Errorf("restored member %s differs from product lineage", processID)
		}
	}
	for processID, snapshot := range snapshots {
		if snapshot.Status().Terminal() {
			continue
		}
		if _, survives := members[processID]; !survives {
			return nil, fmt.Errorf("active Process %s has no surviving product member", processID)
		}
	}
	return members, nil
}

func (session *interactionSession) restoreDelegateCalls(
	snapshots map[agent.ProcessID]agent.Snapshot,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[delegateCallIdentity]*managedDelegateCall, map[agent.ProcessID]*managedDelegateCall, error) {
	session.mu.Lock()
	deployments := session.deployments
	session.mu.Unlock()
	if deployments == nil {
		return nil, nil, errors.New("native Interaction deployments are unavailable")
	}
	calls := make(map[delegateCallIdentity]*managedDelegateCall)
	children := make(map[agent.ProcessID]*managedDelegateCall)
	for parentID, parentSnapshot := range snapshots {
		active, found, err := interaction.ActiveDelegateChildrenFromSnapshot(parentSnapshot)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect parent %s: %w", parentID, err)
		}
		if !found {
			continue
		}
		for _, child := range active {
			managedCall, survives, err := restoreManagedDelegateCall(
				deployments, snapshots, members, parentID, parentSnapshot, child,
			)
			if err != nil {
				return nil, nil, err
			}
			if !survives {
				continue
			}
			calls[managedCall.identity] = managedCall
			children[child.ProcessID()] = managedCall
		}
	}
	for processID := range members {
		if snapshots[processID].Relation().IsRoot() {
			continue
		}
		if children[processID] == nil {
			return nil, nil, fmt.Errorf("surviving child %s has no active Delegate attribution", processID)
		}
	}
	return calls, children, nil
}

func restoreManagedDelegateCall(
	deployments *interactionDeploymentSet,
	snapshots map[agent.ProcessID]agent.Snapshot,
	members map[agent.ProcessID]runs.WaitingMember,
	parentID agent.ProcessID,
	parentSnapshot agent.Snapshot,
	child interaction.ActiveDelegateChild,
) (*managedDelegateCall, bool, error) {
	childSnapshot, exists := snapshots[child.ProcessID()]
	if !exists {
		return nil, false, fmt.Errorf("delegate child %s is absent from the tree", child.ProcessID())
	}
	relation := childSnapshot.Relation()
	relationParent, hasParent := relation.ParentID()
	relationKey, hasKey := relation.ChildKey()
	if !hasParent || !hasKey || relationParent != parentID || relationKey != child.ChildKey() {
		return nil, false, fmt.Errorf("delegate child %s relation differs from interaction state", child.ProcessID())
	}
	member, survives := members[child.ProcessID()]
	if !survives {
		if !childSnapshot.Status().Terminal() {
			return nil, false, fmt.Errorf("delegate child %s has no surviving run binding", child.ProcessID())
		}
		return nil, false, nil
	}
	target, managed := deployments.delegateTarget(parentSnapshot.DeploymentRef(), child.ToolCall().Name)
	if !managed || target != childSnapshot.DeploymentRef() {
		return nil, false, fmt.Errorf("delegate child %s changed exact deployment", child.ProcessID())
	}
	input, arguments, err := decodeDelegateCall(child.ToolCall())
	if err != nil {
		return nil, false, fmt.Errorf("decode Delegate child %s input: %w", child.ProcessID(), err)
	}
	binding := runs.ChildRunBinding{
		MemberID: child.ProcessID().String(), RunID: member.RunID, ParentRunID: member.ParentRunID,
	}
	if err := binding.Validate(); err != nil {
		return nil, false, err
	}
	return &managedDelegateCall{
		identity:          delegateCallIdentity{parentID: parentID, childKey: child.ChildKey()},
		parentRelation:    parentSnapshot.Relation(),
		target:            target,
		call:              child.ToolCall(),
		input:             input,
		arguments:         arguments,
		modelCallSequence: child.ModelCallSequence(),
		toolCallIndex:     child.ToolCallIndex(),
		callID: delegatedToolCallID(
			parentSnapshot.Relation(), child.ModelCallSequence(), child.ToolCallIndex(), child.ToolCall(),
		),
		binding: binding, childProcessID: child.ProcessID(), toolStarted: true,
	}, true, nil
}

func restoreInteractionAccounting(
	total accounting.Snapshot,
	checkpoint interactionCheckpointState,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[agent.ProcessID]map[string]accounting.ModelUsage, map[string]accounting.ModelUsage, error) {
	if err := total.Validate(); err != nil {
		return nil, nil, err
	}
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(members))
	activeAggregate := make(map[string]accounting.ModelUsage)
	for processID, member := range members {
		usage, err := accountingFromRunMetrics(member.Metrics, checkpoint.callsByProcess[processID])
		if err != nil {
			return nil, nil, fmt.Errorf("member %s: %w", processID, err)
		}
		usageByProcess[processID] = usage
		mergeInteractionUsage(activeAggregate, usage)
	}
	carried, err := subtractInteractionUsage(total, activeAggregate)
	if err != nil {
		return nil, nil, err
	}
	expectedCarriedCalls := make(map[string]int)
	for model, calls := range checkpoint.carriedCallCount {
		expectedCarriedCalls[model] = calls
	}
	for processID, byModel := range checkpoint.callsByProcess {
		if _, active := members[processID]; active {
			continue
		}
		for model, calls := range byModel {
			expectedCarriedCalls[model] += calls
		}
	}
	for model, usage := range carried {
		if expectedCarriedCalls[model] != usage.Calls {
			return nil, nil, fmt.Errorf("carried model %q call count differs from checkpoint", model)
		}
		delete(expectedCarriedCalls, model)
	}
	for model, calls := range expectedCarriedCalls {
		if calls != 0 {
			return nil, nil, fmt.Errorf("carried model %q has calls without aggregate usage", model)
		}
	}
	return usageByProcess, carried, nil
}

func accountingFromRunMetrics(
	metrics transcript.RunMetrics,
	callsByModel map[string]int,
) (map[string]accounting.ModelUsage, error) {
	if err := metrics.Validate(); err != nil {
		return nil, err
	}
	if metrics.Steps == 0 {
		if metrics.Usage != nil || len(callsByModel) != 0 {
			return nil, errors.New("zero-step member has accounting state")
		}
		return map[string]accounting.ModelUsage{}, nil
	}
	if metrics.Usage == nil || len(metrics.Usage.ByModel) == 0 {
		return nil, errors.New("model calls have no per-model usage")
	}
	result := make(map[string]accounting.ModelUsage, len(metrics.Usage.ByModel))
	for model, value := range metrics.Usage.ByModel {
		calls := callsByModel[model]
		if calls <= 0 {
			return nil, fmt.Errorf("model %q has no durable call count", model)
		}
		usage := accounting.ModelUsage{
			Model: model,
			TokenUsage: accounting.TokenUsage{
				PromptTokens: value.InputTokens, CompletionTokens: value.OutputTokens,
				ReasoningTokens: value.ReasoningTokens, CacheReadTokens: value.CacheReadTokens,
				CacheWriteTokens: value.CacheWriteTokens,
			},
			Calls: calls,
		}
		if value.CostUSD != nil {
			usage.CostUSD = *value.CostUSD
		}
		if err := usage.Validate(); err != nil {
			return nil, fmt.Errorf("model %q: %w", model, err)
		}
		result[model] = usage
	}
	if len(result) != len(callsByModel) {
		return nil, errors.New("durable call counts name a model absent from product metrics")
	}
	models := make([]accounting.ModelUsage, 0, len(result))
	for _, usage := range result {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		if left.Model < right.Model {
			return -1
		}
		if left.Model > right.Model {
			return 1
		}
		return 0
	})
	total, err := (accounting.Snapshot{Models: models}).Total()
	if err != nil {
		return nil, err
	}
	if total.Calls != metrics.Steps || !sameTranscriptUsage(total, metrics.Usage.ModelUsage) {
		return nil, errors.New("product metrics differ from reconstructed executor accounting")
	}
	return result, nil
}

func sameTranscriptUsage(total accounting.ModelUsage, value transcript.ModelUsage) bool {
	cost := 0.0
	if value.CostUSD != nil {
		cost = *value.CostUSD
	}
	return total.PromptTokens == value.InputTokens && total.CompletionTokens == value.OutputTokens &&
		total.ReasoningTokens == value.ReasoningTokens && total.CacheReadTokens == value.CacheReadTokens &&
		total.CacheWriteTokens == value.CacheWriteTokens && sameInteractionCost(total.CostUSD, cost)
}

func subtractInteractionUsage(
	total accounting.Snapshot,
	active map[string]accounting.ModelUsage,
) (map[string]accounting.ModelUsage, error) {
	remaining := make(map[string]accounting.ModelUsage, len(total.Models))
	for _, usage := range total.Models {
		remaining[usage.Model] = usage
	}
	for model, used := range active {
		value, found := remaining[model]
		if !found || value.PromptTokens < used.PromptTokens ||
			value.CompletionTokens < used.CompletionTokens || value.ReasoningTokens < used.ReasoningTokens ||
			value.CacheReadTokens < used.CacheReadTokens || value.CacheWriteTokens < used.CacheWriteTokens ||
			value.Calls < used.Calls || value.CostUSD+1e-9 < used.CostUSD {
			return nil, fmt.Errorf("active model %q usage exceeds tree checkpoint", model)
		}
		value.PromptTokens -= used.PromptTokens
		value.CompletionTokens -= used.CompletionTokens
		value.ReasoningTokens -= used.ReasoningTokens
		value.CacheReadTokens -= used.CacheReadTokens
		value.CacheWriteTokens -= used.CacheWriteTokens
		value.Calls -= used.Calls
		value.CostUSD -= used.CostUSD
		if math.Abs(value.CostUSD) < 1e-9 {
			value.CostUSD = 0
		}
		if value.Calls == 0 {
			if value.TokenUsage != (accounting.TokenUsage{}) || value.CostUSD != 0 {
				return nil, fmt.Errorf("model %q has residual usage without calls", model)
			}
			delete(remaining, model)
			continue
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		remaining[model] = value
	}
	return remaining, nil
}

func sameInteractionCost(left, right float64) bool {
	return math.Abs(left-right) <= 1e-9*math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
}
