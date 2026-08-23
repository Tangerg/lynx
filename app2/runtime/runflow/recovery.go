package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const lostRunDetail = "the Runtime stopped before the active execution reached a durable settlement"

type TreeRecoveryRunWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Depth             uint32
	Items             []transcript.Record
	Events            []rundomain.EventRecord
}

type TreeRecoveryWrite struct {
	Runs     []TreeRecoveryRunWrite
	Messages []conversationdomain.Record
}

type recoveryTreeMember struct {
	record rundomain.Record
	depth  uint32
}

type recoveryTreeProjection struct {
	writes   []TreeRecoveryRunWrite
	messages []conversationdomain.Record
	events   []protocol.RunEvent
	root     rundomain.Record
	stream   treeStream
}

// Recover settles every predecessor-owned root tree exactly once. Active
// effects are not replayable because they may already have changed the world;
// complete waiting trees are absent from this query and remain resumable.
func (service *Service) Recover(ctx context.Context) error {
	records, err := service.store.ListRunningRuns(ctx)
	if err != nil {
		return fmt.Errorf("runflow: list predecessor executions: %w", err)
	}
	rootIDs := make([]string, 0)
	seen := make(map[string]bool)
	for _, record := range records {
		rootID := record.Run.RootRunID()
		if rootID == "" {
			rootID = record.Run.ID()
		}
		if !seen[rootID] {
			seen[rootID] = true
			rootIDs = append(rootIDs, rootID)
		}
	}
	for _, rootID := range rootIDs {
		lock := service.runLock(rootID)
		lock.Lock()
		err = service.recoverTree(ctx, rootID)
		lock.Unlock()
		if errors.Is(err, rundomain.ErrInvalidTransition) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) settleUnlaunched(runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	record, err := service.store.GetRun(ctx, runID)
	if err != nil {
		return
	}
	rootID := record.Run.RootRunID()
	if rootID == "" {
		rootID = record.Run.ID()
	}
	_ = service.recoverTree(ctx, rootID)
}

func (service *Service) recoverTree(ctx context.Context, rootRunID string) error {
	root, err := service.store.GetRun(ctx, rootRunID)
	if err != nil {
		return err
	}
	records, err := service.store.ListOpenRunTree(ctx, rootRunID)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	if root.Run.Status() != rundomain.Running {
		for _, record := range records {
			if record.Run.Status() == rundomain.Running {
				return errors.New("runflow: running child survived an inactive root")
			}
		}
		return nil
	}
	members, err := recoveryTreeMembers(root, records)
	if err != nil {
		return err
	}
	durableRoot := members[len(members)-1].record
	stream, err := newTreeStream(durableRoot.Run.ID(), durableRoot.Run.ActiveSegmentID())
	if err != nil {
		return err
	}
	projection, err := service.projectLostTree(ctx, members, stream, service.now().UTC())
	if err != nil {
		return err
	}
	if err := service.store.CommitTreeRecovery(ctx, TreeRecoveryWrite{
		Runs: projection.writes, Messages: projection.messages,
	}); err != nil {
		return err
	}
	for _, write := range projection.writes {
		service.publishLifecycleChange(ctx, write.Run.Run)
	}
	service.publishInterruptChange(projection.root.Run)
	for _, event := range projection.events {
		service.hub.PublishRun(projection.stream.rootRunID, projection.stream.rootSegmentID, event)
	}
	return nil
}

func recoveryTreeMembers(
	root rundomain.Record,
	records []rundomain.Record,
) ([]recoveryTreeMember, error) {
	byID := make(map[string]rundomain.Record, len(records))
	for _, record := range records {
		_, duplicate := byID[record.Run.ID()]
		if record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() == "" ||
			record.Run.SessionID() != root.Run.SessionID() || duplicate {
			return nil, errors.New("runflow: recovery requires one fully running Run tree")
		}
		byID[record.Run.ID()] = record
	}
	durableRoot, found := byID[root.Run.ID()]
	if !found || durableRoot.Run.ParentRunID() != "" || durableRoot.Run.RootRunID() != "" {
		return nil, errors.New("runflow: recovery tree has no exact root")
	}
	depths := make(map[string]uint32, len(records))
	visiting := make(map[string]bool, len(records))
	var depth func(string) (uint32, error)
	depth = func(runID string) (uint32, error) {
		if value, found := depths[runID]; found {
			return value, nil
		}
		if visiting[runID] {
			return 0, errors.New("runflow: recovery Run tree contains a cycle")
		}
		visiting[runID] = true
		record, found := byID[runID]
		if !found {
			return 0, errors.New("runflow: recovery Run parent is missing")
		}
		value := uint32(0)
		if record.Run.ParentRunID() != "" {
			if record.Run.RootRunID() != root.Run.ID() {
				return 0, errors.New("runflow: recovery child changed root lineage")
			}
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
	members := make([]recoveryTreeMember, 0, len(records))
	for _, record := range records {
		value, err := depth(record.Run.ID())
		if err != nil {
			return nil, err
		}
		members = append(members, recoveryTreeMember{record: record, depth: value})
	}
	sort.Slice(members, func(left, right int) bool {
		if members[left].depth != members[right].depth {
			return members[left].depth > members[right].depth
		}
		return members[left].record.Run.ID() < members[right].record.Run.ID()
	})
	if members[len(members)-1].record.Run.ID() != root.Run.ID() || members[len(members)-1].depth != 0 {
		return nil, errors.New("runflow: recovery root is not last")
	}
	return members, nil
}

func (service *Service) projectLostTree(
	ctx context.Context,
	members []recoveryTreeMember,
	stream treeStream,
	now time.Time,
) (recoveryTreeProjection, error) {
	lostDelegateItems := make(map[string]bool, len(members)-1)
	for _, member := range members {
		if member.record.Run.ParentRunID() != "" {
			lostDelegateItems[member.record.Run.SpawnedByItemID()] = true
		}
	}
	projection := recoveryTreeProjection{stream: stream}
	var rootItems map[string]transcript.Record
	for _, member := range members {
		record := member.record
		segmentID := record.Run.ActiveSegmentID()
		facts, err := decodeFacts(record.Body)
		if err != nil {
			return recoveryTreeProjection{}, err
		}
		storedItems, err := service.store.ListItems(ctx, "", record.Run.ID())
		if err != nil {
			return recoveryTreeProjection{}, err
		}
		itemsByID := make(map[string]transcript.Record, len(storedItems))
		updated := make([]transcript.Record, 0)
		events := make([]protocol.RunEvent, 0)
		for _, stored := range storedItems {
			var item protocol.Item
			if err := json.Unmarshal(stored.Body, &item); err != nil {
				return recoveryTreeProjection{}, fmt.Errorf("runflow: decode recovery Item %s: %w", stored.ID, err)
			}
			if item.Status == protocol.ItemStatusRunning {
				problemType := protocol.ProblemToolCanceled
				if lostDelegateItems[item.ID] {
					problemType = protocol.ProblemToolFailed
				}
				if err := interruptRunningItem(&item, problemType, lostRunDetail, now); err != nil {
					return recoveryTreeProjection{}, err
				}
				stored.Body, err = json.Marshal(item)
				if err != nil {
					return recoveryTreeProjection{}, err
				}
				updated = append(updated, stored)
				completed, err := service.event(record.Run.ID(), segmentID, &facts, protocol.StreamEvent{
					Type: protocol.StreamItemCompleted, Item: &item,
				}, now)
				if err != nil {
					return recoveryTreeProjection{}, err
				}
				events = append(events, completed)
			}
			itemsByID[stored.ID] = stored
		}
		if err := record.Run.Finish(segmentID, rundomain.Lost, lostRunDetail, now); err != nil {
			return recoveryTreeProjection{}, err
		}
		problem := &protocol.ProblemData{Type: protocol.ProblemRunLost, Detail: lostRunDetail}
		finished, err := service.event(record.Run.ID(), segmentID, &facts, protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished,
			Outcome: segmentOutcome(rundomain.Lost, problem, ""), Metrics: &facts.Metrics,
		}, now)
		if err != nil {
			return recoveryTreeProjection{}, err
		}
		events = append(events, finished)
		record, err = makeRecord(record.Run, facts)
		if err != nil {
			return recoveryTreeProjection{}, err
		}
		persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1, stream)
		if err != nil {
			return recoveryTreeProjection{}, err
		}
		projection.writes = append(projection.writes, TreeRecoveryRunWrite{
			Run: record, ExpectedSegmentID: segmentID, Depth: member.depth,
			Items: updated, Events: persisted,
		})
		projection.events = append(projection.events, events...)
		if member.depth == 0 {
			projection.root = record
			rootItems = itemsByID
		}
	}
	conversation, err := service.store.ListConversationMessages(ctx, projection.root.Run.SessionID())
	if err != nil {
		return recoveryTreeProjection{}, err
	}
	projection.messages, err = projectConversation(
		projection.root, agentexec.Output{}, rootItems, conversation,
	)
	if err != nil {
		return recoveryTreeProjection{}, err
	}
	return projection, nil
}
