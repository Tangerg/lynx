package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const lostRunDetail = "the Runtime stopped before the active execution reached a durable settlement"

// Recover settles executions owned by a predecessor Runtime instance. A
// running Agent effect cannot be replayed safely: it may already have changed
// the world. Recovery therefore records lost exactly once under the old
// segment generation, while complete waiting checkpoints remain resumable.
func (service *Service) Recover(ctx context.Context) error {
	records, err := service.store.ListRunningRuns(ctx)
	if err != nil {
		return fmt.Errorf("runflow: list predecessor executions: %w", err)
	}
	for _, candidate := range records {
		lock := service.runLock(candidate.Run.ID())
		lock.Lock()
		err = service.recoverRun(ctx, candidate.Run.ID())
		lock.Unlock()
		if errors.Is(err, rundomain.ErrInvalidTransition) {
			// Another Runtime generation won the same compare-and-set.
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
	_ = service.recoverRun(ctx, runID)
}

func (service *Service) recoverRun(ctx context.Context, runID string) error {
	record, err := service.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if record.Run.Status() != rundomain.Running {
		return nil
	}
	segmentID := record.Run.ActiveSegmentID()
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return err
	}
	now := service.now().UTC()
	items, messages, events, err := service.lostMaterial(ctx, record, segmentID, &facts, now)
	if err != nil {
		return err
	}
	if err := record.Run.Finish(segmentID, rundomain.Lost, lostRunDetail, now); err != nil {
		return err
	}
	problem := &protocol.ProblemData{Type: protocol.ProblemRunLost, Detail: lostRunDetail}
	finished, err := service.event(runID, segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentFinished,
		Outcome: segmentOutcome(rundomain.Lost, problem, ""),
		Metrics: &facts.Metrics,
	}, now)
	if err != nil {
		return err
	}
	events = append(events, finished)
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return err
	}
	persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1)
	if err != nil {
		return err
	}
	if err := service.store.CommitRun(ctx, CommitWrite{
		Run: record, ExpectedStatus: rundomain.Running, ExpectedSegmentID: segmentID,
		Items: items, Messages: messages, Events: persisted,
	}); err != nil {
		return err
	}
	service.publishLifecycleChange(record.Run)
	for _, event := range events { service.hub.PublishRun(event) }
	return nil
}

func (service *Service) lostMaterial(
	ctx context.Context,
	record rundomain.Record,
	segmentID string,
	facts *runFacts,
	now time.Time,
) ([]transcript.Record, []conversationdomain.Record, []protocol.RunEvent, error) {
	storedItems, err := service.store.ListItems(ctx, "", record.Run.ID())
	if err != nil {
		return nil, nil, nil, err
	}
	open := make(map[string]protocol.Item)
	byItemID := make(map[string]transcript.Record)
	for _, stored := range storedItems {
		var item protocol.Item
		if err := json.Unmarshal(stored.Body, &item); err != nil {
			return nil, nil, nil, fmt.Errorf("runflow: decode recovery item %s: %w", stored.ID, err)
		}
		if item.Status != protocol.ItemStatusRunning {
			continue
		}
		if item.Type != protocol.ItemTypeToolCall || item.Tool == nil {
			return nil, nil, nil, fmt.Errorf("runflow: running run %s owns unsupported open item %s", record.Run.ID(), item.ID)
		}
		open[item.ID] = item
		byItemID[item.ID] = stored
	}
	if len(open) == 0 {
		return nil, nil, nil, nil
	}

	journal, err := service.store.ListConversationMessages(ctx, record.Run.SessionID())
	if err != nil {
		return nil, nil, nil, err
	}
	calls := make(map[string]chat.ToolCall, len(open))
	for _, stored := range journal {
		var message chat.Message
		if err := json.Unmarshal(stored.Body, &message); err != nil {
			return nil, nil, nil, fmt.Errorf("runflow: decode recovery conversation message: %w", err)
		}
		if message.Role != chat.RoleAssistant {
			continue
		}
		for _, part := range message.Parts {
			if part.Kind != chat.PartToolCall || part.ToolCall == nil {
				continue
			}
			itemID := agentexec.ToolItemID(record.Run.ID(), part.ToolCall.ID)
			if _, found := open[itemID]; found {
				calls[itemID] = *part.ToolCall
			}
		}
	}

	updated := make([]transcript.Record, 0, len(open))
	results := make([]chat.ToolResult, 0, len(open))
	events := make([]protocol.RunEvent, 0, len(open))
	for _, stored := range storedItems {
		item, found := open[stored.ID]
		if !found {
			continue
		}
		call, found := calls[item.ID]
		if !found {
			return nil, nil, nil, fmt.Errorf("runflow: open tool item %s has no conversation ToolCall", item.ID)
		}
		item.Status = protocol.ItemStatusIncomplete
		item.FinishedAt = now
		item.DurationMillis = nil
		item.Tool.Result = nil
		item.Error = &protocol.ProblemData{Type: protocol.ProblemToolCanceled, Detail: lostRunDetail}
		body, err := json.Marshal(item)
		if err != nil {
			return nil, nil, nil, err
		}
		value := byItemID[item.ID]
		value.Body = body
		updated = append(updated, value)
		results = append(results, chat.ToolResult{ID: call.ID, Name: call.Name, Result: lostRunDetail, IsError: true})
		event, err := service.event(record.Run.ID(), segmentID, facts, protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}, now)
		if err != nil {
			return nil, nil, nil, err
		}
		events = append(events, event)
	}
	if len(results) == 0 {
		return updated, nil, events, nil
	}
	message := chat.NewToolMessage(results...)
	body, err := json.Marshal(message)
	if err != nil {
		return nil, nil, nil, err
	}
	messages := []conversationdomain.Record{{
		SessionID: record.Run.SessionID(), RunID: record.Run.ID(),
		Ordinal: nextConversationOrdinal(journal), Body: slices.Clone(body),
	}}
	return updated, messages, events, nil
}
