package runflow

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/domain/delegation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
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

type DelegationStore interface {
	CommitDelegateAdmission(context.Context, DelegateAdmissionWrite) (delegation.Admission, bool, error)
	CommitDelegateStart(context.Context, DelegateStartWrite) (bool, error)
	CommitDelegateAbort(context.Context, DelegateAbortWrite) (bool, error)
	GetDelegateAdmission(context.Context, string) (delegation.Admission, error)
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
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal)
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
		service.hub.PublishRun(event)
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
		Provider: admission.Provider, Model: admission.Model, Now: admission.StartedAt,
	})
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
	events, persisted, err := service.startEvents(child.ID(), admission.SegmentID, *presented, nil, admission.StartedAt)
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
		service.publishLifecycleChange(child)
		service.hub.PublishRun(events[0])
	}
	return delegateBinding(admission), nil
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
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal)
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
		service.hub.PublishRun(event)
	}
	return delegateBinding(admission), nil
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
