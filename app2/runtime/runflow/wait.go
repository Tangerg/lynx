package runflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type WaitWrite struct {
	Run         rundomain.Record
	ExpectedSegmentID string
	Items       []transcript.Record
	Messages    []conversationdomain.Record
	ToolResults []toolresult.Record
	Interrupts  protocol.PendingInterruptSet
	Checkpoint  []byte
	Events      []rundomain.EventRecord
}

func (service *Service) parkExecution(ctx context.Context, record rundomain.Record, facts runFacts, segmentID string, waiting agentexec.Waiting, tools []agentexec.ToolObservation, projection executionProjection, now time.Time) error {
	stream, err := service.activeTreeStream(ctx, record.Run, segmentID)
	if err != nil {
		return err
	}
	var prompt agentexec.ToolInputPrompt
	if err := json.Unmarshal(waiting.Prompt, &prompt); err != nil {
		return fmt.Errorf("decode tool input prompt: %w", err)
	}
	item, interrupt, err := waitingProjection(record.Run.ID(), prompt, now)
	if err != nil {
		return err
	}
	for _, observation := range tools {
		if observation.ItemID == item.ID && item.Type == protocol.ItemTypeToolCall {
			item.StartedAt = observation.StartedAt
			break
		}
	}
	existing, err := service.store.ListItems(ctx, "", record.Run.ID())
	if err != nil {
		return err
	}
	allItems := append(slices.Clone(existing), projection.items...)
	storedItem, existed := transcriptRecord(existing, item.ID)
	if existed {
		var current protocol.Item
		if err := json.Unmarshal(storedItem.Body, &current); err != nil {
			return err
		}
		if current.Status != protocol.ItemStatusRunning || current.Type != item.Type {
			return errors.New("runflow: waiting Item conflicts with its live projection")
		}
		storedItem.Body, err = json.Marshal(item)
		if err != nil {
			return err
		}
	} else {
		storedItem, err = itemRecord(record.Run.SessionID(), item, nextOrdinal(allItems, record.Run.ID()))
		if err != nil {
			return err
		}
	}
	set := protocol.PendingInterruptSet{
		RootRunID: record.Run.ID(), SessionID: record.Run.SessionID(),
		Interrupts: []protocol.Interrupt{interrupt}, CreatedAt: now,
	}
	if err := record.Run.Wait(segmentID, now); err != nil {
		return err
	}
	events := slices.Clone(projection.events)
	if !existed {
		started, eventErr := service.event(record.Run.ID(), segmentID, &facts, protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}, now)
		if eventErr != nil { return eventErr }
		events = append(events, started)
	}
	event, err := service.event(record.Run.ID(), segmentID, &facts, protocol.StreamEvent{
		Type: protocol.StreamSegmentFinished,
		Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentInterrupt, Interrupts: set.Interrupts},
		Metrics: &facts.Metrics,
	}, now)
	if err != nil {
		return err
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return err
	}
	events = append(events, event)
	persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1, stream)
	if err != nil {
		return err
	}
	items := append(projection.items, storedItem)
	if err := service.store.CommitWait(ctx, WaitWrite{Run: record, ExpectedSegmentID: segmentID, Items: items, Messages: projection.messages, ToolResults: projection.results, Interrupts: set, Checkpoint: waiting.Checkpoint, Events: persisted}); err != nil {
		return err
	}
	service.publishLifecycleChange(record.Run)
	service.publishInterruptChange(record.Run)
	for _, event := range events {
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	return nil
}

func waitingProjection(runID string, prompt agentexec.ToolInputPrompt, now time.Time) (protocol.Item, protocol.Interrupt, error) {
	if strings.TrimSpace(prompt.ItemID) == "" {
		return protocol.Item{}, protocol.Interrupt{}, errors.New("runflow: waiting prompt has no item identity")
	}
	switch prompt.Kind {
	case "approval":
		if prompt.Tool == nil || strings.TrimSpace(prompt.Tool.Name) == "" {
			return protocol.Item{}, protocol.Interrupt{}, errors.New("runflow: approval prompt has no tool")
		}
		invocation := &protocol.ToolInvocation{Name: prompt.Tool.Name, Arguments: cloneMap(prompt.Tool.Arguments)}
		item := protocol.Item{
			ID: prompt.ItemID, RunID: runID, Status: protocol.ItemStatusRunning,
			StartedAt: now, Type: protocol.ItemTypeToolCall, Tool: invocation, SafetyClass: prompt.SafetyClass,
		}
		interrupt := protocol.Interrupt{
			ItemID: prompt.ItemID, RunID: runID, Type: protocol.InterruptApproval,
			Payload: &protocol.InterruptPayload{Tool: invocation, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable},
		}
		return item, interrupt, nil
	case "question":
		if prompt.Question == nil {
			return protocol.Item{}, protocol.Interrupt{}, errors.New("runflow: question prompt has no question")
		}
		question := *prompt.Question
		item := protocol.Item{ID: prompt.ItemID, RunID: runID, Status: protocol.ItemStatusRunning, CreatedAt: now, Type: protocol.ItemTypeQuestion, Question: &question}
		interrupt := protocol.Interrupt{ItemID: prompt.ItemID, RunID: runID, Type: protocol.InterruptQuestion, Payload: &protocol.InterruptPayload{Question: &question}}
		return item, interrupt, nil
	default:
		return protocol.Item{}, protocol.Interrupt{}, fmt.Errorf("runflow: unknown tool input kind %q", prompt.Kind)
	}
}
