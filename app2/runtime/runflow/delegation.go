package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	"github.com/Tangerg/lynx/app2/runtime/domain/delegation"
	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// DelegateAdmissionWrite makes the model-authored parent ToolCall and its
// private prospective-child reservation visible as one fact. Created reports
// whether this call installed the fact or observed an exact replay.
type DelegateAdmissionWrite struct {
	Admission         delegation.Admission
	Parent            rundomain.Record
	ExpectedSegmentID string
	Item              transcript.Record
	Event             rundomain.EventRecord
}

// DelegateStartWrite is the only transition that turns a private admission
// into a Lyra child Run. The child and its segment.started event are created in
// the same transaction that concludes the admission.
type DelegateStartWrite struct {
	Admission delegation.Admission
	Child     rundomain.Record
	Event     rundomain.EventRecord
}

// DelegateAbortWrite concludes initialization failure without inventing a
// child Run. The parent ToolCall becomes incomplete in the same transaction.
type DelegateAbortWrite struct {
	Admission         delegation.Admission
	Parent            rundomain.Record
	ExpectedSegmentID string
	Item              transcript.Record
	Event             rundomain.EventRecord
}

// DelegateCompletionWrite settles one child and the parent ToolCall that owns
// its result. Both Run journals advance together so lineage can never expose a
// terminal child behind a still-running Delegate item, or the reverse.
type DelegateCompletionWrite struct {
	Child                   rundomain.Record
	ExpectedChildSegmentID  string
	ChildItems              []transcript.Record
	ChildToolResults        []toolresult.Record
	ChildEvents             []rundomain.EventRecord
	Parent                  rundomain.Record
	ExpectedParentSegmentID string
	ParentItem              transcript.Record
	ParentEvent             rundomain.EventRecord
	ParentMessages          []conversationdomain.Record
}

type DelegationStore interface {
	CommitDelegateAdmission(context.Context, DelegateAdmissionWrite) (delegation.Admission, bool, error)
	CommitDelegateStart(context.Context, DelegateStartWrite) (bool, error)
	CommitDelegateAbort(context.Context, DelegateAbortWrite) (bool, error)
	CommitDelegateCompletion(context.Context, DelegateCompletionWrite) error
	GetDelegateAdmission(context.Context, string) (delegation.Admission, error)
	ListDelegateAdmissions(context.Context, string) ([]delegation.Admission, error)
}

func (service *Service) ReserveDelegate(
	ctx context.Context,
	request agentexec.DelegateRequest,
) (agentexec.DelegateBinding, error) {
	if err := request.Validate(); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	lock := service.runLock(request.ParentRunID)
	lock.Lock()
	defer lock.Unlock()
	if existing, err := service.store.GetDelegateAdmission(ctx, request.MemberID); err == nil {
		return bindingForRequest(existing, request)
	} else if !errors.Is(err, delegation.ErrNotFound) {
		return agentexec.DelegateBinding{}, err
	}
	parent, err := service.store.GetRun(ctx, request.ParentRunID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if parent.Run.Status() != rundomain.Running || parent.Run.ActiveSegmentID() != request.ParentSegmentID {
		return agentexec.DelegateBinding{}, rundomain.ErrStaleSegment
	}
	facts, err := decodeFacts(parent.Body)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if !runAllowsSubagents(parent.Run, facts.Profile) {
		return agentexec.DelegateBinding{}, errors.New("runflow: delegated child was not negotiated")
	}
	stream, err := service.activeTreeStream(ctx, parent.Run, request.ParentSegmentID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	runID, err := service.ids.New("run_")
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	segmentID, err := service.ids.New("seg_")
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	itemID := agentexec.ToolItemID(parent.Run.ID(), request.CallID)
	rootRunID := parent.Run.RootRunID()
	if rootRunID == "" {
		rootRunID = parent.Run.ID()
	}
	reserved, err := delegation.New(delegation.Reserve{
		MemberID: request.MemberID, ParentMemberID: request.ParentMemberID, ChildKey: request.ChildKey,
		RunID: runID, SegmentID: segmentID, SessionID: parent.Run.SessionID(),
		ParentRunID: parent.Run.ID(), RootRunID: rootRunID, SpawnedByItemID: itemID,
		Provider: parent.Run.Provider(), Model: parent.Run.Model(),
		Summary: request.Task.Summary, Instructions: request.Task.Instructions,
		StartedAt: request.StartedAt,
	})
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	existingItems, err := service.store.ListItems(ctx, "", parent.Run.ID())
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	item := protocol.Item{
		ID: itemID, RunID: parent.Run.ID(), Status: protocol.ItemStatusRunning,
		StartedAt: request.RequestedAt.UTC(), Type: protocol.ItemTypeToolCall,
		Tool: &protocol.ToolInvocation{Name: agentexec.DelegateToolName, Arguments: map[string]any{
			"summary": request.Task.Summary, "instructions": request.Task.Instructions,
		}},
		SafetyClass: protocol.SafetyClassExec,
	}
	storedItem, err := itemRecord(parent.Run.SessionID(), item, nextOrdinal(existingItems, parent.Run.ID()))
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	now := service.now().UTC()
	if err := parent.Run.Touch(request.ParentSegmentID, now); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	event, err := service.event(parent.Run.ID(), request.ParentSegmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamItemStarted, Item: &item,
	}, item.StartedAt)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	parent, err = makeRecord(parent.Run, facts)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal, stream)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	committed, created, err := service.store.CommitDelegateAdmission(ctx, DelegateAdmissionWrite{
		Admission: reserved, Parent: parent, ExpectedSegmentID: request.ParentSegmentID,
		Item: storedItem, Event: persisted[0],
	})
	if errors.Is(err, delegation.ErrAdmissionConflict) {
		if authoritative, lookupErr := service.store.GetDelegateAdmission(ctx, request.MemberID); lookupErr == nil {
			return bindingForRequest(authoritative, request)
		}
	}
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if created {
		service.publishRunChange(parent.Run)
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	return delegateBinding(committed), nil
}

func (service *Service) ConcludeDelegateStart(
	ctx context.Context,
	outcome agentexec.DelegateStartOutcome,
) (agentexec.DelegateBinding, error) {
	if outcome.MemberID == "" || outcome.Started == (outcome.Failure != "") {
		return agentexec.DelegateBinding{}, errors.New("runflow: invalid delegate start outcome")
	}
	admission, err := service.store.GetDelegateAdmission(ctx, outcome.MemberID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	lock := service.runLock(admission.ParentRunID)
	lock.Lock()
	defer lock.Unlock()
	if outcome.Started {
		return service.commitDelegateStarted(ctx, admission)
	}
	return service.commitDelegateAborted(ctx, admission, outcome.Failure)
}

func (service *Service) commitDelegateStarted(
	ctx context.Context,
	admission delegation.Admission,
) (agentexec.DelegateBinding, error) {
	if admission.Status == delegation.Started {
		return delegateBinding(admission), nil
	}
	if err := admission.MarkStarted(service.now().UTC()); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	child, err := rundomain.New(rundomain.Start{
		ID: admission.RunID, SessionID: admission.SessionID, SegmentID: admission.SegmentID,
		ParentRunID: admission.ParentRunID, RootRunID: admission.RootRunID,
		SpawnedByItemID: admission.SpawnedByItemID,
		Provider:        admission.Provider, Model: admission.Model, Now: admission.StartedAt,
	})
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	stream, err := service.activeTreeStream(ctx, child, admission.SegmentID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	record, err := makeRecord(child, runFacts{Metrics: protocol.RunMetrics{}, EventOrdinal: 1})
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	presented, err := presentRecord(record)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	events, persisted, err := service.startEvents(
		child.ID(),
		admission.SegmentID,
		stream,
		*presented,
		nil,
		admission.StartedAt,
	)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	created, err := service.store.CommitDelegateStart(ctx, DelegateStartWrite{
		Admission: admission, Child: record, Event: persisted[0],
	})
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if created {
		service.publishLifecycleChange(ctx, child)
		service.observeSubagentStarted(ctx, child, admission)
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, events[0])
	}
	return delegateBinding(admission), nil
}

func (service *Service) observeSubagentStarted(
	ctx context.Context,
	child rundomain.Run,
	admission delegation.Admission,
) {
	storedSession, err := service.store.GetSession(ctx, session.ID(child.SessionID()))
	if err != nil {
		return
	}
	prompt, promptTruncated := boundedHookMaterial(
		admission.Instructions,
		lifecyclehook.MaxPromptBytes,
	)
	service.hooks.Observe(ctx, lifecyclehook.Invocation{
		Event:     lifecyclehook.SubagentStart,
		SessionID: child.SessionID(), RunID: child.ID(),
		Workspace: storedSession.Workspace().Path(),
		Subagent: &lifecyclehook.SubagentInput{
			RunID: child.ID(), ParentRunID: child.ParentRunID(),
			Description: boundedHookText(admission.Summary, lifecyclehook.MaxReasonBytes),
			Prompt:      prompt, PromptTruncated: promptTruncated,
		},
	})
}

func (service *Service) commitDelegateAborted(
	ctx context.Context,
	admission delegation.Admission,
	failure string,
) (agentexec.DelegateBinding, error) {
	if admission.Status == delegation.Aborted {
		if admission.Failure != failure {
			return agentexec.DelegateBinding{}, delegation.ErrAdmissionConflict
		}
		return delegateBinding(admission), nil
	}
	parent, err := service.store.GetRun(ctx, admission.ParentRunID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	segmentID := parent.Run.ActiveSegmentID()
	if parent.Run.Status() != rundomain.Running || segmentID == "" {
		return agentexec.DelegateBinding{}, rundomain.ErrInvalidTransition
	}
	stream, err := service.activeTreeStream(ctx, parent.Run, segmentID)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	facts, err := decodeFacts(parent.Body)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	items, err := service.store.ListItems(ctx, "", parent.Run.ID())
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	stored, found := transcriptRecord(items, admission.SpawnedByItemID)
	if !found {
		return agentexec.DelegateBinding{}, errors.New("runflow: delegated parent Item is missing")
	}
	var item protocol.Item
	if err := json.Unmarshal(stored.Body, &item); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	now := service.now().UTC()
	item.Status = protocol.ItemStatusIncomplete
	item.FinishedAt = now
	item.Error = &protocol.ProblemData{Type: protocol.ProblemToolFailed, Detail: failure}
	stored.Body, err = json.Marshal(item)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if err := admission.MarkAborted(failure, now); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if err := parent.Run.Touch(segmentID, now); err != nil {
		return agentexec.DelegateBinding{}, err
	}
	event, err := service.event(parent.Run.ID(), segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamItemCompleted, Item: &item,
	}, now)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	parent, err = makeRecord(parent.Run, facts)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal, stream)
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	committed, err := service.store.CommitDelegateAbort(ctx, DelegateAbortWrite{
		Admission: admission, Parent: parent, ExpectedSegmentID: segmentID,
		Item: stored, Event: persisted[0],
	})
	if err != nil {
		return agentexec.DelegateBinding{}, err
	}
	if committed {
		service.publishRunChange(parent.Run)
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	return delegateBinding(admission), nil
}

func (service *Service) finishDelegatedExecutions(ctx context.Context, outputs []agentexec.ChildOutput) error {
	for _, output := range outputs {
		if err := service.finishDelegatedExecution(ctx, output); err != nil {
			return fmt.Errorf("settle delegated Run %s: %w", output.RunID, err)
		}
	}
	return nil
}

func (service *Service) finishDelegatedExecution(ctx context.Context, output agentexec.ChildOutput) error {
	if output.RunID == "" || output.SegmentID == "" || output.ParentRunID == "" ||
		output.RootRunID == "" || output.StartedAt.IsZero() || output.FinishedAt.IsZero() ||
		output.FinishedAt.Before(output.StartedAt) {
		return errors.New("runflow: delegated execution output is incomplete")
	}
	outcome, problem, err := delegatedOutcome(output)
	if err != nil {
		return err
	}
	for _, model := range output.Models {
		if model.RunID != output.RunID || model.SegmentID != output.SegmentID {
			return errors.New("runflow: delegated model material changed source Run")
		}
	}
	for _, tool := range output.Tools {
		if tool.RunID != output.RunID || tool.SegmentID != output.SegmentID {
			return errors.New("runflow: delegated Tool material changed source Run")
		}
	}
	unlock := service.lockRunPair(output.RunID, output.ParentRunID)
	defer unlock()
	child, err := service.store.GetRun(ctx, output.RunID)
	if err != nil {
		return err
	}
	if child.Run.Status() == rundomain.Finished {
		return nil
	}
	if child.Run.Status() != rundomain.Running || child.Run.ActiveSegmentID() != output.SegmentID ||
		child.Run.ParentRunID() != output.ParentRunID || child.Run.RootRunID() != output.RootRunID {
		return rundomain.ErrInvalidTransition
	}
	parent, err := service.store.GetRun(ctx, output.ParentRunID)
	if err != nil {
		return err
	}
	parentRootRunID := parent.Run.RootRunID()
	if parentRootRunID == "" {
		parentRootRunID = parent.Run.ID()
	}
	if child.Run.SessionID() != parent.Run.SessionID() || parentRootRunID != output.RootRunID {
		return errors.New("runflow: delegated execution changed tree lineage")
	}
	parentSegmentID := parent.Run.ActiveSegmentID()
	if parent.Run.Status() != rundomain.Running || parentSegmentID == "" {
		return rundomain.ErrInvalidTransition
	}
	stream, err := service.activeTreeStream(ctx, child.Run, output.SegmentID)
	if err != nil {
		return err
	}
	childFacts, err := decodeFacts(child.Body)
	if err != nil {
		return err
	}
	mergeRunUsage(&childFacts.Metrics, output.Usage, output.ModelCalls)
	if output.ContextTokens > 0 {
		childFacts.ContextTokens = output.ContextTokens
	}
	projection, err := service.projectExecution(ctx, child, output.SegmentID, agentexec.Output{
		Text: output.Reply, Usage: output.Usage, ModelCalls: output.ModelCalls,
		ContextTokens: output.ContextTokens, Models: output.Models, Tools: output.Tools,
	}, &childFacts, executionProjectionPolicy{terminal: true})
	if err != nil {
		return err
	}
	finishedAt := output.FinishedAt.UTC()
	if err := child.Run.Finish(output.SegmentID, outcome, output.Detail, service.now().UTC()); err != nil {
		return err
	}
	childFinished, err := service.event(child.Run.ID(), output.SegmentID, &childFacts, protocol.StreamEvent{
		Type:    protocol.StreamSegmentFinished,
		Outcome: segmentOutcome(outcome, problem, output.Detail), Metrics: &childFacts.Metrics,
	}, finishedAt)
	if err != nil {
		return err
	}
	projection.events = append(projection.events, childFinished)
	child, err = makeRecord(child.Run, childFacts)
	if err != nil {
		return err
	}
	childEvents, err := persistEvents(
		projection.events,
		childFacts.EventOrdinal-len(projection.events)+1,
		stream,
	)
	if err != nil {
		return err
	}
	parentFacts, err := decodeFacts(parent.Body)
	if err != nil {
		return err
	}
	parentItems, err := service.store.ListItems(ctx, "", parent.Run.ID())
	if err != nil {
		return err
	}
	parentStored, found := transcriptRecord(parentItems, child.Run.SpawnedByItemID())
	if !found {
		return errors.New("runflow: delegated parent Item is missing")
	}
	var parentItem protocol.Item
	if err := json.Unmarshal(parentStored.Body, &parentItem); err != nil {
		return err
	}
	if parentItem.RunID != parent.Run.ID() || parentItem.Type != protocol.ItemTypeToolCall || parentItem.Tool == nil ||
		parentItem.Tool.Name != agentexec.DelegateToolName ||
		parentItem.Status != protocol.ItemStatusRunning {
		return errors.New("runflow: delegated parent Item is not running")
	}
	parentItem.FinishedAt = finishedAt
	duration := output.FinishedAt.Sub(output.StartedAt).Milliseconds()
	if duration >= 0 {
		parentItem.DurationMillis = &duration
	}
	if outcome == rundomain.Completed {
		parentItem.Status = protocol.ItemStatusCompleted
		parentItem.Tool.Result = map[string]any{"runId": child.Run.ID(), "reply": output.Reply}
		parentItem.Error = nil
	} else {
		parentItem.Status = protocol.ItemStatusIncomplete
		problemType := protocol.ProblemToolFailed
		if outcome == rundomain.Canceled {
			problemType = protocol.ProblemChildRunCanceled
		}
		parentItem.Tool.Result = nil
		parentItem.Error = &protocol.ProblemData{Type: problemType, Detail: output.Detail}
	}
	parentStored.Body, err = json.Marshal(parentItem)
	if err != nil {
		return err
	}
	parentMessages := []conversationdomain.Record(nil)
	if parent.Run.ParentRunID() == "" {
		conversation, err := service.store.ListConversationMessages(ctx, parent.Run.SessionID())
		if err != nil {
			return err
		}
		byID := make(map[string]transcript.Record, len(parentItems))
		for _, item := range parentItems {
			byID[item.ID] = item
		}
		byID[parentStored.ID] = parentStored
		parentMessages, err = projectConversation(parent, agentexec.Output{}, byID, conversation)
		if err != nil {
			return err
		}
	}
	if err := parent.Run.Touch(parentSegmentID, service.now().UTC()); err != nil {
		return err
	}
	parentCompleted, err := service.event(parent.Run.ID(), parentSegmentID, &parentFacts, protocol.StreamEvent{
		Type: protocol.StreamItemCompleted, Item: &parentItem,
	}, finishedAt)
	if err != nil {
		return err
	}
	parent, err = makeRecord(parent.Run, parentFacts)
	if err != nil {
		return err
	}
	parentEvents, err := persistEvents(
		[]protocol.RunEvent{parentCompleted},
		parentFacts.EventOrdinal,
		stream,
	)
	if err != nil {
		return err
	}
	if err := service.store.CommitDelegateCompletion(ctx, DelegateCompletionWrite{
		Child: child, ExpectedChildSegmentID: output.SegmentID,
		ChildItems: projection.items, ChildToolResults: projection.results, ChildEvents: childEvents,
		Parent: parent, ExpectedParentSegmentID: parentSegmentID,
		ParentItem: parentStored, ParentEvent: parentEvents[0], ParentMessages: parentMessages,
	}); err != nil {
		return err
	}
	service.publishLifecycleChangeWithResult(ctx, child.Run, output.Reply)
	service.publishRunChange(parent.Run)
	for _, event := range projection.events {
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, parentCompleted)
	return nil
}

func delegatedOutcome(output agentexec.ChildOutput) (rundomain.Outcome, *protocol.ProblemData, error) {
	switch output.Status {
	case agentexec.ChildCompleted:
		return rundomain.Completed, nil, nil
	case agentexec.ChildTimedOut:
		return rundomain.TimedOut, nil, nil
	case agentexec.ChildCanceled, agentexec.ChildKilled:
		return rundomain.Canceled, nil, nil
	case agentexec.ChildFailed:
		problem := modelFailureProblem(output.ModelFailure)
		if problem == nil {
			problem = &protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: output.Detail}
		}
		return rundomain.Failed, problem, nil
	default:
		return "", nil, errors.New("runflow: delegated execution has invalid terminal status")
	}
}

func (service *Service) lockRunPair(left, right string) func() {
	if right < left {
		left, right = right, left
	}
	first := service.runLock(left)
	second := service.runLock(right)
	first.Lock()
	second.Lock()
	return func() {
		second.Unlock()
		first.Unlock()
	}
}

func bindingForRequest(
	value delegation.Admission,
	request agentexec.DelegateRequest,
) (agentexec.DelegateBinding, error) {
	if value.MemberID != request.MemberID || value.ParentMemberID != request.ParentMemberID ||
		value.ChildKey != request.ChildKey || value.ParentRunID != request.ParentRunID ||
		value.Summary != request.Task.Summary || value.Instructions != request.Task.Instructions ||
		value.SpawnedByItemID != agentexec.ToolItemID(request.ParentRunID, request.CallID) ||
		!value.StartedAt.Equal(request.StartedAt.UTC()) {
		return agentexec.DelegateBinding{}, delegation.ErrAdmissionConflict
	}
	return delegateBinding(value), nil
}

func delegateBinding(value delegation.Admission) agentexec.DelegateBinding {
	return agentexec.DelegateBinding{
		RunID: value.RunID, SegmentID: value.SegmentID,
		ParentRunID: value.ParentRunID, RootRunID: value.RootRunID,
	}
}

func runAllowsSubagents(value rundomain.Run, profile protocol.RunProtocolProfile) bool {
	if value.ParentRunID() != "" {
		return true
	}
	for _, feature := range profile.RequiredFeatures {
		if feature == protocol.RunProtocolFeatureSubagents {
			return true
		}
	}
	return false
}

var _ agentexec.DelegationCoordinator = (*Service)(nil)
