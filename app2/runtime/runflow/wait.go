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
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type WaitWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Items             []transcript.Record
	Messages          []conversationdomain.Record
	ToolResults       []toolresult.Record
	Interrupts        protocol.PendingInterruptSet
	Checkpoint        []byte
	Events            []rundomain.EventRecord
}

// TreeWaitRunWrite is one source-owned member of an atomic Run-tree pause.
// Writes are ordered descendants first and the root last, matching the event
// stream's causal close order.
type TreeWaitRunWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Depth             uint32
	Items             []transcript.Record
	Messages          []conversationdomain.Record
	ToolResults       []toolresult.Record
	Events            []rundomain.EventRecord
}

// TreeWaitWrite is the single durable boundary between an executing tree and
// a resumable HITL set. The checkpoint and interrupt set belong to the root;
// transcript, metrics, and Segment outcomes remain owned by their source Runs.
type TreeWaitWrite struct {
	Runs       []TreeWaitRunWrite
	Interrupts protocol.PendingInterruptSet
	Checkpoint []byte
}

func (service *Service) parkTreeExecution(
	ctx context.Context,
	root rundomain.Record,
	rootSegmentID string,
	waiting agentexec.Waiting,
	now time.Time,
) error {
	if !waiting.Tree || len(waiting.Checkpoint) == 0 || len(waiting.Runs) == 0 {
		return errors.New("runflow: waiting tree checkpoint is incomplete")
	}
	if root.Run.ParentRunID() != "" || root.Run.Status() != rundomain.Running ||
		root.Run.ActiveSegmentID() != rootSegmentID {
		return errors.New("runflow: waiting tree has no active root")
	}
	stream, err := newTreeStream(root.Run.ID(), rootSegmentID)
	if err != nil {
		return err
	}
	writes := make([]TreeWaitRunWrite, 0, len(waiting.Runs))
	published := make([]protocol.RunEvent, 0, len(waiting.Runs)*2)
	interrupts := make([]protocol.Interrupt, 0)
	seenRuns := make(map[string]bool, len(waiting.Runs))
	rootCount := 0
	lastDepth := ^uint32(0)
	for index, source := range waiting.Runs {
		if source.RunID == "" || source.SegmentID == "" || source.RootRunID != root.Run.ID() ||
			seenRuns[source.RunID] || source.Depth > lastDepth {
			return errors.New("runflow: waiting tree order or identity is invalid")
		}
		lastDepth = source.Depth
		seenRuns[source.RunID] = true
		record, err := service.store.GetRun(ctx, source.RunID)
		if err != nil {
			return err
		}
		if record.Run.SessionID() != root.Run.SessionID() || record.Run.Status() != rundomain.Running ||
			record.Run.ActiveSegmentID() != source.SegmentID || record.Run.ParentRunID() != source.ParentRunID {
			return errors.New("runflow: waiting tree differs from durable Run lineage")
		}
		if source.RunID == root.Run.ID() {
			rootCount++
			if source.Depth != 0 || source.ParentRunID != "" || index != len(waiting.Runs)-1 {
				return errors.New("runflow: waiting tree root must close last")
			}
		} else if source.Depth == 0 || record.Run.RootRunID() != root.Run.ID() {
			return errors.New("runflow: waiting child has invalid root lineage")
		}
		if err := validateWaitingMaterial(source); err != nil {
			return err
		}
		facts, err := decodeFacts(record.Body)
		if err != nil {
			return err
		}
		mergeRunUsage(&facts.Metrics, source.Usage, source.ModelCalls)
		if source.ContextTokens > 0 {
			facts.ContextTokens = source.ContextTokens
		}
		projection, err := service.projectExecution(
			ctx,
			record,
			source.SegmentID,
			agentexec.Output{
				Usage: source.Usage, ModelCalls: source.ModelCalls,
				ContextTokens: source.ContextTokens, Models: source.Models, Tools: source.Tools,
			},
			&facts,
			executionProjectionPolicy{sessionConversation: source.RunID == root.Run.ID()},
		)
		if err != nil {
			return err
		}
		events := slices.Clone(projection.events)
		items := slices.Clone(projection.items)
		memberInterrupts := make([]protocol.Interrupt, 0, 1)
		switch source.Disposition {
		case agentexec.WaitingInterrupt:
			item, interrupt, stored, existed, err := service.prepareTreeInterrupt(ctx, record, source, projection.items, now)
			if err != nil {
				return err
			}
			if !existed {
				startedAt := item.StartedAt
				if startedAt.IsZero() {
					startedAt = item.CreatedAt
				}
				if startedAt.IsZero() {
					startedAt = now
				}
				started, err := service.event(source.RunID, source.SegmentID, &facts, protocol.StreamEvent{
					Type: protocol.StreamItemStarted, Item: &item,
				}, startedAt)
				if err != nil {
					return err
				}
				events = append(events, started)
			}
			items = append(items, stored)
			memberInterrupts = append(memberInterrupts, interrupt)
			interrupts = append(interrupts, interrupt)
		case agentexec.WaitingSuspended:
			if len(source.Prompt) != 0 || len(source.ResponseSchema) != 0 {
				return errors.New("runflow: suspended Run carries interrupt material")
			}
		default:
			return errors.New("runflow: waiting Run has an unknown disposition")
		}
		if err := record.Run.Wait(source.SegmentID, now); err != nil {
			return err
		}
		outcome := &protocol.SegmentOutcome{Type: protocol.SegmentSuspended}
		if source.Disposition == agentexec.WaitingInterrupt {
			outcome = &protocol.SegmentOutcome{Type: protocol.SegmentInterrupt, Interrupts: memberInterrupts}
		}
		finished, err := service.event(source.RunID, source.SegmentID, &facts, protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished, Outcome: outcome, Metrics: &facts.Metrics,
		}, now)
		if err != nil {
			return err
		}
		events = append(events, finished)
		record, err = makeRecord(record.Run, facts)
		if err != nil {
			return err
		}
		persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1, stream)
		if err != nil {
			return err
		}
		writes = append(writes, TreeWaitRunWrite{
			Run: record, ExpectedSegmentID: source.SegmentID, Depth: source.Depth,
			Items: items, Messages: projection.messages, ToolResults: projection.results, Events: persisted,
		})
		published = append(published, events...)
	}
	if rootCount != 1 || len(interrupts) == 0 {
		return errors.New("runflow: waiting tree needs one root and at least one interrupt")
	}
	set := protocol.PendingInterruptSet{
		RootRunID: root.Run.ID(), SessionID: root.Run.SessionID(),
		Interrupts: interrupts, CreatedAt: now,
	}
	if err := service.store.CommitTreeWait(ctx, TreeWaitWrite{
		Runs: writes, Interrupts: set, Checkpoint: waiting.Checkpoint,
	}); err != nil {
		return err
	}
	for _, write := range writes {
		service.publishLifecycleChange(ctx, write.Run.Run)
	}
	service.publishInterruptChange(writes[len(writes)-1].Run.Run)
	for _, event := range published {
		service.hub.PublishRun(stream.rootRunID, stream.rootSegmentID, event)
	}
	for index, source := range waiting.Runs {
		if source.Disposition == agentexec.WaitingInterrupt {
			service.observeWaitingHook(ctx, writes[index].Run.Run)
		}
	}
	return nil
}

func validateWaitingMaterial(source agentexec.WaitingRun) error {
	if source.ModelCalls != len(source.Models) {
		return errors.New("runflow: waiting Run model-call count is inconsistent")
	}
	for _, model := range source.Models {
		if model.RunID != source.RunID || model.SegmentID != source.SegmentID {
			return errors.New("runflow: waiting model material changed source Run")
		}
	}
	for _, tool := range source.Tools {
		if tool.RunID != source.RunID || tool.SegmentID != source.SegmentID {
			return errors.New("runflow: waiting Tool material changed source Run")
		}
	}
	return nil
}

func (service *Service) prepareTreeInterrupt(
	ctx context.Context,
	record rundomain.Record,
	source agentexec.WaitingRun,
	projected []transcript.Record,
	now time.Time,
) (protocol.Item, protocol.Interrupt, transcript.Record, bool, error) {
	var prompt agentexec.ToolInputPrompt
	if err := json.Unmarshal(source.Prompt, &prompt); err != nil {
		return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, fmt.Errorf("decode tree tool input prompt: %w", err)
	}
	item, interrupt, err := waitingProjection(source.RunID, prompt, now)
	if err != nil {
		return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, err
	}
	for _, observation := range source.Tools {
		if observation.ItemID == item.ID && item.Type == protocol.ItemTypeToolCall {
			item.StartedAt = observation.StartedAt
			break
		}
	}
	existing, err := service.store.ListItems(ctx, "", source.RunID)
	if err != nil {
		return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, err
	}
	stored, existed := transcriptRecord(existing, item.ID)
	if existed {
		var current protocol.Item
		if err := json.Unmarshal(stored.Body, &current); err != nil {
			return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, err
		}
		if current.Status != protocol.ItemStatusRunning || current.Type != item.Type {
			return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, errors.New("runflow: tree interrupt conflicts with its live Item")
		}
		stored.Body, err = json.Marshal(item)
		if err != nil {
			return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, err
		}
	} else {
		allItems := append(slices.Clone(existing), projected...)
		stored, err = itemRecord(record.Run.SessionID(), item, nextOrdinal(allItems, source.RunID))
		if err != nil {
			return protocol.Item{}, protocol.Interrupt{}, transcript.Record{}, false, err
		}
	}
	return item, interrupt, stored, existed, nil
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
		if eventErr != nil {
			return eventErr
		}
		events = append(events, started)
	}
	event, err := service.event(record.Run.ID(), segmentID, &facts, protocol.StreamEvent{
		Type:    protocol.StreamSegmentFinished,
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
	service.publishLifecycleChange(ctx, record.Run)
	service.observeWaitingHook(ctx, record.Run)
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
		if err := protocol.ValidateWireTree(question); err != nil {
			return protocol.Item{}, protocol.Interrupt{}, fmt.Errorf("runflow: invalid question prompt: %w", err)
		}
		item := protocol.Item{ID: prompt.ItemID, RunID: runID, Status: protocol.ItemStatusRunning, CreatedAt: now, Type: protocol.ItemTypeQuestion, Question: &question}
		interrupt := protocol.Interrupt{ItemID: prompt.ItemID, RunID: runID, Type: protocol.InterruptQuestion, Payload: &protocol.InterruptPayload{Question: &question}}
		return item, interrupt, nil
	default:
		return protocol.Item{}, protocol.Interrupt{}, fmt.Errorf("runflow: unknown tool input kind %q", prompt.Kind)
	}
}
