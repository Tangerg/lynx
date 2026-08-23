package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sort"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	"github.com/Tangerg/lynx/app2/runtime/domain/delegation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type resumedTreeMember struct {
	record    rundomain.Record
	memberID  string
	segmentID string
	depth     uint32
}

func (service *Service) resumeTree(
	ctx context.Context,
	command ResumeCommand,
	root rundomain.Record,
	pending protocol.PendingInterruptSet,
	answers map[string]protocol.InterruptResponseValue,
	allItems, updatedItems []transcript.Record,
	checkpoint []byte,
) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error) {
	request := command.Request
	if len(request.Input) > 0 && len(pending.Interrupts) != 1 {
		return nil, nil, fmt.Errorf("%w: resume input requires one exact interrupted branch", protocol.ErrInvalidParams)
	}
	members, err := service.resumedTreeMembers(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	responses, err := treeResumeResponses(pending, answers, members)
	if err != nil {
		return nil, nil, err
	}
	for index := range members {
		members[index].segmentID, err = service.ids.New("seg_")
		if err != nil {
			return nil, nil, err
		}
	}
	rootMember := members[len(members)-1]
	if rootMember.record.Run.ID() != root.Run.ID() || rootMember.depth != 0 {
		return nil, nil, errors.New("runflow: resumed tree root is not last")
	}
	now := service.now().UTC()
	var opening *transcript.Record
	var openingMessage *conversationdomain.Record
	var userItemID *string
	if len(request.Input) > 0 {
		id, err := service.ids.New("itm_")
		if err != nil {
			return nil, nil, err
		}
		item := protocol.Item{
			ID: id, RunID: root.Run.ID(), Status: protocol.ItemStatusCompleted,
			CreatedAt: now, Type: protocol.ItemTypeUserMessage, Content: slices.Clone(request.Input),
		}
		stored, err := itemRecord(root.Run.SessionID(), item, nextOrdinal(allItems, root.Run.ID()))
		if err != nil {
			return nil, nil, err
		}
		opening = &stored
		userItemID = &id
		message, err := agentexec.UserMessage(request.Input)
		if err != nil {
			return nil, nil, err
		}
		body, err := json.Marshal(message)
		if err != nil {
			return nil, nil, err
		}
		conversation, err := service.store.ListConversationMessages(ctx, root.Run.SessionID())
		if err != nil {
			return nil, nil, err
		}
		value := conversationdomain.Record{
			SessionID: root.Run.SessionID(), RunID: root.Run.ID(),
			Ordinal: nextConversationOrdinal(conversation), Body: body,
		}
		openingMessage = &value
	}
	stream, err := newTreeStream(root.Run.ID(), rootMember.segmentID)
	if err != nil {
		return nil, nil, err
	}
	writes := make([]TreeResumeRunWrite, 0, len(members))
	bindings := make([]agentexec.TreeResumeMember, 0, len(members))
	events := make([]protocol.RunEvent, 0, len(members)+1)
	var committedRoot rundomain.Record
	for _, member := range members {
		record := member.record
		if err := record.Run.Resume(member.segmentID, now); err != nil {
			return nil, nil, err
		}
		facts, err := decodeFacts(record.Body)
		if err != nil {
			return nil, nil, err
		}
		record, err = makeRecord(record.Run, facts)
		if err != nil {
			return nil, nil, err
		}
		presented, err := presentRecord(record)
		if err != nil {
			return nil, nil, err
		}
		started, err := service.event(record.Run.ID(), member.segmentID, &facts, protocol.StreamEvent{
			Type: protocol.StreamSegmentStarted, Run: presented,
		}, now)
		if err != nil {
			return nil, nil, err
		}
		memberEvents := []protocol.RunEvent{started}
		if record.Run.ID() == root.Run.ID() && opening != nil {
			var item protocol.Item
			if err := json.Unmarshal(opening.Body, &item); err != nil {
				return nil, nil, err
			}
			completed, err := service.event(record.Run.ID(), member.segmentID, &facts, protocol.StreamEvent{
				Type: protocol.StreamItemCompleted, Item: &item,
			}, now)
			if err != nil {
				return nil, nil, err
			}
			memberEvents = append(memberEvents, completed)
		}
		record, err = makeRecord(record.Run, facts)
		if err != nil {
			return nil, nil, err
		}
		persisted, err := persistEvents(
			memberEvents,
			facts.EventOrdinal-len(memberEvents)+1,
			stream,
		)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, TreeResumeRunWrite{Run: record, Depth: member.depth, Events: persisted})
		bindings = append(bindings, agentexec.TreeResumeMember{
			MemberID: member.memberID, RunID: record.Run.ID(), SegmentID: member.segmentID,
			ParentRunID: record.Run.ParentRunID(), RootRunID: root.Run.ID(), Depth: member.depth,
		})
		events = append(events, memberEvents...)
		if record.Run.ID() == root.Run.ID() {
			committedRoot = record
		}
	}
	if err := service.store.CommitTreeResume(ctx, TreeResumeWrite{
		Runs: writes, ExpectedInterrupts: pending, UpdatedItems: updatedItems,
		OpeningItem: opening, OpeningMessage: openingMessage,
	}); err != nil {
		if errors.Is(err, ErrInterruptSetNotFound) {
			return nil, nil, protocol.ErrInterruptNotOpen
		}
		return nil, nil, err
	}
	for _, write := range writes {
		service.publishLifecycleChange(write.Run.Run)
	}
	service.publishInterruptChange(committedRoot.Run)
	if command.BeforeLaunch != nil {
		if err := command.BeforeLaunch(ctx, root.Run.ID()); err != nil {
			service.settleUnlaunched(root.Run.ID())
			return nil, nil, fmt.Errorf("runflow: prepare resumed Run-tree ownership: %w", err)
		}
	}
	storedSession, err := service.store.GetSession(ctx, session.ID(root.Run.SessionID()))
	if err != nil {
		return nil, nil, err
	}
	streamEvents := service.hub.SubscribeRun(ctx, root.Run.ID(), rootMember.segmentID, events)
	if !service.launchResumedExecution(committedRoot, rootMember.segmentID, storedSession.Workspace().Path(), agentexec.ResumeInput{
		Checkpoint: checkpoint, TreeMembers: bindings, TreeResponses: responses,
		AdditionalInput: slices.Clone(request.Input),
	}) {
		service.settleUnlaunched(root.Run.ID())
	}
	return &protocol.ResumeRunResponse{
		RunID: root.Run.ID(), SegmentID: rootMember.segmentID, UserItemID: userItemID,
	}, streamEvents, nil
}

func (service *Service) resumedTreeMembers(
	ctx context.Context,
	root rundomain.Record,
) ([]resumedTreeMember, error) {
	records, err := service.store.ListOpenRunTree(ctx, root.Run.ID())
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("runflow: waiting Run tree is empty")
	}
	byID := make(map[string]rundomain.Record, len(records))
	for _, record := range records {
		if record.Run.Status() != rundomain.Waiting || record.Run.SessionID() != root.Run.SessionID() {
			return nil, errors.New("runflow: resume requires a fully waiting Run tree")
		}
		byID[record.Run.ID()] = record
	}
	if durable, found := byID[root.Run.ID()]; !found || durable.Run.ParentRunID() != "" {
		return nil, errors.New("runflow: waiting tree has no durable root")
	}
	admissions, err := service.store.ListDelegateAdmissions(ctx, root.Run.ID())
	if err != nil {
		return nil, err
	}
	byRun := make(map[string]delegation.Admission, len(admissions))
	for _, admission := range admissions {
		byRun[admission.RunID] = admission
	}
	depths := make(map[string]uint32, len(records))
	visiting := make(map[string]bool, len(records))
	var depth func(string) (uint32, error)
	depth = func(runID string) (uint32, error) {
		if value, found := depths[runID]; found {
			return value, nil
		}
		if visiting[runID] {
			return 0, errors.New("runflow: waiting Run tree contains a cycle")
		}
		visiting[runID] = true
		record, found := byID[runID]
		if !found {
			return 0, errors.New("runflow: waiting Run parent is missing")
		}
		value := uint32(0)
		if record.Run.ParentRunID() != "" {
			parentDepth, err := depth(record.Run.ParentRunID())
			if err != nil {
				return 0, err
			}
			value = parentDepth + 1
		}
		delete(visiting, runID)
		depths[runID] = value
		return value, nil
	}
	members := make([]resumedTreeMember, 0, len(records))
	for _, record := range records {
		valueDepth, err := depth(record.Run.ID())
		if err != nil {
			return nil, err
		}
		memberID := ""
		if valueDepth > 0 {
			admission, found := byRun[record.Run.ID()]
			if !found || admission.Status != delegation.Started ||
				admission.ParentRunID != record.Run.ParentRunID() || admission.RootRunID != root.Run.ID() {
				return nil, errors.New("runflow: waiting child has no exact executor admission")
			}
			memberID = admission.MemberID
		}
		members = append(members, resumedTreeMember{
			record: record, memberID: memberID, depth: valueDepth,
		})
	}
	sort.Slice(members, func(left, right int) bool {
		if members[left].depth != members[right].depth {
			return members[left].depth > members[right].depth
		}
		return members[left].record.Run.ID() < members[right].record.Run.ID()
	})
	return members, nil
}

func treeResumeResponses(
	pending protocol.PendingInterruptSet,
	answers map[string]protocol.InterruptResponseValue,
	members []resumedTreeMember,
) ([]agentexec.TreeResumeResponse, error) {
	knownRuns := make(map[string]bool, len(members))
	for _, member := range members {
		knownRuns[member.record.Run.ID()] = true
	}
	seenRuns := make(map[string]bool, len(pending.Interrupts))
	responses := make([]agentexec.TreeResumeResponse, 0, len(pending.Interrupts))
	for _, interrupt := range pending.Interrupts {
		if !knownRuns[interrupt.RunID] || seenRuns[interrupt.RunID] {
			return nil, errors.New("runflow: interrupt set does not name exact waiting tree members")
		}
		payload, err := frameworkInterruptResponse(interrupt, answers[interrupt.ItemID])
		if err != nil {
			return nil, err
		}
		seenRuns[interrupt.RunID] = true
		responses = append(responses, agentexec.TreeResumeResponse{RunID: interrupt.RunID, Payload: payload})
	}
	return responses, nil
}
