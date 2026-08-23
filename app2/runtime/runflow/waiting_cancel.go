package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type WaitingTreeCancelRunWrite struct {
	Run   rundomain.Record
	Depth uint32
}

type WaitingTreeCancelWrite struct {
	Runs               []WaitingTreeCancelRunWrite
	ExpectedInterrupts protocol.PendingInterruptSet
	Items              []transcript.Record
	Messages           []conversationdomain.Record
	Events             []rundomain.EventRecord
}

type waitingCancelState struct {
	member resumedTreeMember
	record rundomain.Record
	facts  runFacts
}

type waitingCancelProjection struct {
	writes   []WaitingTreeCancelRunWrite
	items    []transcript.Record
	messages []conversationdomain.Record
	events   []protocol.RunEvent
	persisted []rundomain.EventRecord
	bindings []agentexec.TreeResumeMember
	byRun    map[string]rundomain.Record
	root     rundomain.Record
	stream   treeStream
}

func (service *Service) cancelWaitingTreeChild(
	ctx context.Context,
	target rundomain.Record,
	reason string,
) (*protocol.CancelRunResponse, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "canceled by user"
	}
	root, err := service.store.GetRun(ctx, target.Run.RootRunID())
	if err != nil {
		return nil, err
	}
	if root.Run.Status() != rundomain.Waiting || !runUsesSubagents(root.Body) {
		return nil, protocol.ErrSessionBusy
	}
	pending, err := service.store.GetInterruptSet(ctx, root.Run.ID())
	if err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	checkpoint, err := service.store.GetExecutorCheckpoint(ctx, root.Run.ID())
	if err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	members, err := service.resumedTreeMembers(ctx, root)
	if err != nil {
		return nil, err
	}
	storedSession, err := service.store.GetSession(ctx, session.ID(root.Run.SessionID()))
	if err != nil {
		return nil, err
	}
	frameworkMembers := make([]agentexec.WaitingTreeMember, 0, len(members))
	for _, member := range members {
		frameworkMembers = append(frameworkMembers, agentexec.WaitingTreeMember{
			MemberID: member.memberID, RunID: member.record.Run.ID(),
			ParentRunID: member.record.Run.ParentRunID(), RootRunID: root.Run.ID(), Depth: member.depth,
		})
	}
	transformed, err := service.executor.CancelWaitingTree(ctx, agentexec.WaitingTreeCancelInput{
		Provider: root.Run.Provider(), Model: root.Run.Model(), Workspace: storedSession.Workspace().Path(),
		SessionID: root.Run.SessionID(), RootRunID: root.Run.ID(), TargetRunID: target.Run.ID(),
		Reason: reason, MaxSteps: runMaxSteps(root.Body), Checkpoint: checkpoint,
		Members: frameworkMembers, Delegation: service,
	})
	if errors.Is(err, agentexec.ErrWaitingTreeCancelUnavailable) {
		return nil, protocol.ErrSessionBusy
	}
	if err != nil {
		return nil, err
	}
	canceled, err := exactCanceledSubtree(members, target.Run.ID(), transformed.CanceledRunIDs)
	if err != nil {
		return nil, err
	}
	if err := service.checkpoints.Snapshot(
		ctx, root.Run.SessionID(), storedSession.Workspace().Path(), target.Run.ID(),
	); err != nil {
		return nil, fmt.Errorf("runflow: checkpoint waiting subtree cancel: %w", err)
	}
	items, err := service.store.ListItems(ctx, root.Run.SessionID(), "")
	if err != nil {
		return nil, err
	}
	conversation, err := service.store.ListConversationMessages(ctx, root.Run.SessionID())
	if err != nil {
		return nil, err
	}
	projection, err := service.projectWaitingTreeCancel(members, canceled, items, conversation, reason, service.now().UTC())
	if err != nil {
		return nil, err
	}
	if err := service.store.CommitWaitingTreeCancel(ctx, WaitingTreeCancelWrite{
		Runs: projection.writes, ExpectedInterrupts: pending,
		Items: projection.items, Messages: projection.messages, Events: projection.persisted,
	}); err != nil {
		return nil, err
	}
	service.publishWaitingCancel(ctx, projection)
	rootRecord := projection.root
	rootSegmentID := rootRecord.Run.ActiveSegmentID()
	if rootSegmentID == "" {
		return nil, errors.New("runflow: waiting child cancel did not continue root")
	}
	if !service.launchResumedExecution(rootRecord, rootSegmentID, storedSession.Workspace().Path(), agentexec.ResumeInput{
		Checkpoint: transformed.Checkpoint, TreeMembers: projection.bindings, ContinueTree: true,
	}) {
		service.settleUnlaunched(root.Run.ID())
	}
	canceledTarget := projection.byRun[target.Run.ID()]
	presented, err := presentRecord(canceledTarget)
	if err != nil {
		return nil, err
	}
	presentedRoot, err := presentRecord(rootRecord)
	if err != nil {
		return nil, err
	}
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunChild, Run: *presented, RootRun: presentedRoot,
	}, nil
}

func (service *Service) cancelWaitingTreeRoot(
	ctx context.Context,
	root rundomain.Record,
	reason string,
) (*protocol.CancelRunResponse, error) {
	if strings.TrimSpace(reason) == "" {
		reason = "canceled by user"
	}
	pending, err := service.store.GetInterruptSet(ctx, root.Run.ID())
	if err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	if _, err := service.store.GetExecutorCheckpoint(ctx, root.Run.ID()); err != nil {
		return nil, protocol.ErrInterruptNotOpen
	}
	members, err := service.resumedTreeMembers(ctx, root)
	if err != nil {
		return nil, err
	}
	canceled := make(map[string]bool, len(members))
	for _, member := range members {
		canceled[member.record.Run.ID()] = true
	}
	storedSession, err := service.store.GetSession(ctx, session.ID(root.Run.SessionID()))
	if err != nil {
		return nil, err
	}
	if err := service.checkpoints.Snapshot(
		ctx, root.Run.SessionID(), storedSession.Workspace().Path(), root.Run.ID(),
	); err != nil {
		return nil, fmt.Errorf("runflow: checkpoint waiting tree cancel: %w", err)
	}
	items, err := service.store.ListItems(ctx, root.Run.SessionID(), "")
	if err != nil {
		return nil, err
	}
	conversation, err := service.store.ListConversationMessages(ctx, root.Run.SessionID())
	if err != nil {
		return nil, err
	}
	projection, err := service.projectWaitingTreeCancel(members, canceled, items, conversation, reason, service.now().UTC())
	if err != nil {
		return nil, err
	}
	if err := service.store.CommitWaitingTreeCancel(ctx, WaitingTreeCancelWrite{
		Runs: projection.writes, ExpectedInterrupts: pending,
		Items: projection.items, Messages: projection.messages, Events: projection.persisted,
	}); err != nil {
		return nil, err
	}
	service.publishWaitingCancel(ctx, projection)
	presented, err := presentRecord(projection.root)
	if err != nil {
		return nil, err
	}
	return &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: *presented}, nil
}

func exactCanceledSubtree(
	members []resumedTreeMember,
	targetRunID string,
	frameworkRunIDs []string,
) (map[string]bool, error) {
	parent := make(map[string]string, len(members))
	for _, member := range members {
		parent[member.record.Run.ID()] = member.record.Run.ParentRunID()
	}
	expected := make(map[string]bool)
	for runID := range parent {
		for current := runID; current != ""; current = parent[current] {
			if current == targetRunID {
				expected[runID] = true
				break
			}
		}
	}
	actual := make(map[string]bool, len(frameworkRunIDs))
	for _, runID := range frameworkRunIDs {
		if actual[runID] {
			return nil, errors.New("runflow: Framework repeated a canceled Run")
		}
		actual[runID] = true
	}
	if len(expected) != len(actual) {
		return nil, errors.New("runflow: Framework canceled subtree differs from product lineage")
	}
	for runID := range expected {
		if !actual[runID] {
			return nil, errors.New("runflow: Framework canceled subtree differs from product lineage")
		}
	}
	return expected, nil
}

func (service *Service) projectWaitingTreeCancel(
	members []resumedTreeMember,
	canceled map[string]bool,
	storedItems []transcript.Record,
	conversation []conversationdomain.Record,
	reason string,
	now time.Time,
) (waitingCancelProjection, error) {
	if len(members) == 0 || len(canceled) == 0 {
		return waitingCancelProjection{}, errors.New("runflow: waiting cancel projection is empty")
	}
	rootID := members[len(members)-1].record.Run.ID()
	states := make(map[string]*waitingCancelState, len(members))
	for index := range members {
		segmentID, err := service.ids.New("seg_")
		if err != nil {
			return waitingCancelProjection{}, err
		}
		members[index].segmentID = segmentID
		record := members[index].record
		if err := record.Run.Resume(segmentID, now); err != nil {
			return waitingCancelProjection{}, err
		}
		facts, err := decodeFacts(record.Body)
		if err != nil {
			return waitingCancelProjection{}, err
		}
		states[record.Run.ID()] = &waitingCancelState{member: members[index], record: record, facts: facts}
	}
	rootState := states[rootID]
	stream, err := newTreeStream(rootID, rootState.record.Run.ActiveSegmentID())
	if err != nil {
		return waitingCancelProjection{}, err
	}
	projection := waitingCancelProjection{byRun: make(map[string]rundomain.Record, len(members)), stream: stream}
	appendEvent := func(state *waitingCancelState, payload protocol.StreamEvent) error {
		event, err := service.event(
			state.record.Run.ID(), state.member.segmentID, &state.facts, payload, now,
		)
		if err != nil {
			return err
		}
		persisted, err := persistEvents([]protocol.RunEvent{event}, state.facts.EventOrdinal, stream)
		if err != nil {
			return err
		}
		projection.events = append(projection.events, event)
		projection.persisted = append(projection.persisted, persisted[0])
		return nil
	}
	for _, member := range members {
		state := states[member.record.Run.ID()]
		record, err := makeRecord(state.record.Run, state.facts)
		if err != nil {
			return waitingCancelProjection{}, err
		}
		presented, err := presentRecord(record)
		if err != nil {
			return waitingCancelProjection{}, err
		}
		if err := appendEvent(state, protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: presented}); err != nil {
			return waitingCancelProjection{}, err
		}
	}
	items := make(map[string]transcript.Record, len(storedItems))
	material := make(map[string]protocol.Item, len(storedItems))
	for _, stored := range storedItems {
		var item protocol.Item
		if err := json.Unmarshal(stored.Body, &item); err != nil {
			return waitingCancelProjection{}, err
		}
		items[item.ID] = stored
		material[item.ID] = item
	}
	updated := make(map[string]transcript.Record)
	completeItem := func(state *waitingCancelState, item protocol.Item, problemType string) error {
		if item.Status != protocol.ItemStatusRunning {
			return nil
		}
		if err := interruptRunningItem(&item, problemType, reason, now); err != nil {
			return err
		}
		stored, found := items[item.ID]
		if !found || stored.RunID != state.record.Run.ID() {
			return errors.New("runflow: canceled Item ownership is invalid")
		}
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		stored.Body = body
		items[item.ID] = stored
		material[item.ID] = item
		updated[item.ID] = stored
		return appendEvent(state, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item})
	}
	for _, member := range members {
		runID := member.record.Run.ID()
		if !canceled[runID] {
			continue
		}
		state := states[runID]
		for _, stored := range storedItems {
			item := material[stored.ID]
			if item.RunID == runID && item.Status == protocol.ItemStatusRunning {
				if err := completeItem(state, item, protocol.ProblemToolCanceled); err != nil {
					return waitingCancelProjection{}, err
				}
			}
		}
		if err := state.record.Run.Finish(state.record.Run.ActiveSegmentID(), rundomain.Canceled, reason, now); err != nil {
			return waitingCancelProjection{}, err
		}
		if err := appendEvent(state, protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished,
			Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentCanceled, Detail: reason},
			Metrics: &state.facts.Metrics,
		}); err != nil {
			return waitingCancelProjection{}, err
		}
		parentID := member.record.Run.ParentRunID()
		if parentID != "" {
			parent := states[parentID]
			item := material[member.record.Run.SpawnedByItemID()]
			if parent == nil || item.ID == "" || item.RunID != parentID {
				return waitingCancelProjection{}, errors.New("runflow: canceled child has no parent Delegate Item")
			}
			if err := completeItem(parent, item, protocol.ProblemChildRunCanceled); err != nil {
				return waitingCancelProjection{}, err
			}
		}
	}
	projection.items = make([]transcript.Record, 0, len(updated))
	for _, stored := range storedItems {
		if value, found := updated[stored.ID]; found {
			projection.items = append(projection.items, value)
		}
	}
	projection.writes = make([]WaitingTreeCancelRunWrite, 0, len(members))
	projection.bindings = make([]agentexec.TreeResumeMember, 0, len(members)-len(canceled))
	for _, member := range members {
		state := states[member.record.Run.ID()]
		record, err := makeRecord(state.record.Run, state.facts)
		if err != nil {
			return waitingCancelProjection{}, err
		}
		projection.writes = append(projection.writes, WaitingTreeCancelRunWrite{Run: record, Depth: member.depth})
		projection.byRun[record.Run.ID()] = record
		if record.Run.Status() == rundomain.Running {
			projection.bindings = append(projection.bindings, agentexec.TreeResumeMember{
				MemberID: member.memberID, RunID: record.Run.ID(), SegmentID: record.Run.ActiveSegmentID(),
				ParentRunID: record.Run.ParentRunID(), RootRunID: rootID, Depth: member.depth,
			})
		}
	}
	projection.root = projection.byRun[rootID]
	projection.messages, err = projectConversation(
		projection.root, agentexec.Output{}, items, conversation,
	)
	if err != nil {
		return waitingCancelProjection{}, err
	}
	return projection, nil
}

func (service *Service) publishWaitingCancel(ctx context.Context, projection waitingCancelProjection) {
	for _, write := range projection.writes {
		service.publishLifecycleChange(ctx, write.Run.Run)
	}
	service.publishInterruptChange(projection.root.Run)
	for _, event := range projection.events {
		service.hub.PublishRun(projection.stream.rootRunID, projection.stream.rootSegmentID, event)
	}
}
