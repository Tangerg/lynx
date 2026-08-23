package runflow

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const liveProjectionCapacity = 256

// RunEventWrite advances one active Run's replay journal without materializing
// a provisional transcript Item. The stable item.started anchor is replayable;
// the content-bearing item.delta remains live-only.
type RunEventWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Event             rundomain.EventRecord
}

// RunItemEventWrite atomically advances an active Run's replay journal and its
// one durable ToolCall projection. A large result, when present, is owned by the
// same commit so an Item can never point at material that was not stored.
type RunItemEventWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Item              transcript.Record
	ToolResult        *toolresult.Record
	Events            []rundomain.EventRecord
}

type liveProjector struct {
	service   *Service
	runID     string
	segmentID string
	input     chan liveObservation
	done      chan struct{}
	modelStarted map[string]bool
	progress  map[string]*liveRunProgress
}

type liveRunProgress struct {
	usage protocol.Usage
	steps int
}

type liveObservation struct {
	delta    *agentexec.ModelDelta
	progress *agentexec.ModelProgress
	toolStarted *agentexec.ToolObservation
	toolSettled *agentexec.ToolObservation
}

func newLiveProjector(service *Service, record rundomain.Record, segmentID string) *liveProjector {
	var usage protocol.Usage
	steps := 0
	if facts, err := decodeFacts(record.Body); err == nil {
		steps = facts.Metrics.Steps
		if facts.Metrics.Usage != nil {
			usage = cloneLiveUsage(*facts.Metrics.Usage)
		}
	}
	projector := &liveProjector{
		service: service, runID: record.Run.ID(), segmentID: segmentID,
		input: make(chan liveObservation, liveProjectionCapacity),
		done: make(chan struct{}), modelStarted: make(map[string]bool),
		progress: map[string]*liveRunProgress{
			record.Run.ID(): {usage: usage, steps: steps},
		},
	}
	go projector.run()
	return projector
}

// OfferModelDelta is deliberately non-blocking. Provider streaming is never
// allowed to wait on SQLite or a downstream renderer.
func (projector *liveProjector) OfferModelDelta(delta agentexec.ModelDelta) {
	select {
	case projector.input <- liveObservation{delta: &delta}:
	default:
	}
}

func (projector *liveProjector) OfferModelProgress(progress agentexec.ModelProgress) {
	select {
	case projector.input <- liveObservation{progress: &progress}:
	default:
	}
}

func (projector *liveProjector) OfferToolStarted(observation agentexec.ToolObservation) {
	select {
	case projector.input <- liveObservation{toolStarted: &observation}:
	default:
	}
}

func (projector *liveProjector) OfferToolSettled(observation agentexec.ToolObservation) {
	select {
	case projector.input <- liveObservation{toolSettled: &observation}:
	default:
	}
}

func (projector *liveProjector) Close() {
	close(projector.input)
	<-projector.done
}

func (projector *liveProjector) run() {
	defer close(projector.done)
	for observation := range projector.input {
		if observation.delta != nil {
			projector.projectDelta(*observation.delta)
		}
		if observation.progress != nil {
			projector.projectProgress(*observation.progress)
		}
		if observation.toolStarted != nil {
			projector.projectTool(*observation.toolStarted, false)
		}
		if observation.toolSettled != nil {
			projector.projectTool(*observation.toolSettled, true)
		}
	}
}

func (projector *liveProjector) projectDelta(delta agentexec.ModelDelta) {
	if delta.EffectID == "" || delta.Text == "" || delta.Index < 0 {
		return
	}
	runID, segmentID, ok := projector.target(delta.RunID, delta.SegmentID)
	if !ok {
		return
	}
	item, itemDelta, ok := projector.present(runID, delta)
	if !ok {
		return
	}
	if !projector.modelStarted[item.ID] {
		started, committed := projector.commitAnchor(runID, segmentID, item)
		if !committed {
			return
		}
		projector.modelStarted[item.ID] = true
		projector.service.hub.PublishRun(projector.runID, projector.segmentID, started)
	}
	eventID, err := projector.service.ids.New("evt_")
	if err != nil {
		return
	}
	occurredAt := delta.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = projector.service.now().UTC()
	}
	projector.service.hub.PublishRun(projector.runID, projector.segmentID, protocol.RunEvent{
		RunID: runID, SegmentID: segmentID,
		EventID: eventID, Timestamp: occurredAt,
		Event: protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: item.ID, Delta: &itemDelta},
	})
}

func (projector *liveProjector) projectTool(observation agentexec.ToolObservation, settled bool) {
	if observation.IntrinsicInput || observation.ItemID == "" || observation.Name == "" || observation.StartedAt.IsZero() {
		return
	}
	if settled && (observation.Waiting || observation.FinishedAt.IsZero()) {
		return
	}
	runID, segmentID, ok := projector.target(observation.RunID, observation.SegmentID)
	if !ok {
		return
	}
	lock := projector.service.runLock(runID)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	record, err := projector.service.store.GetRun(ctx, runID)
	if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != segmentID {
		return
	}
	existing, err := projector.service.store.ListItems(ctx, "", runID)
	if err != nil {
		return
	}
	stored, existed := transcriptRecord(existing, observation.ItemID)
	if existed && !settled {
		return
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return
	}
	events := make([]protocol.RunEvent, 0, 3)
	if !existed {
		running := runningToolItem(runID, observation)
		stored, err = itemRecord(
			record.Run.SessionID(),
			running,
			nextOrdinal(existing, runID),
		)
		if err != nil {
			return
		}
		started, eventErr := projector.service.event(
			runID,
			segmentID,
			&facts,
			protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &running},
			observation.StartedAt,
		)
		if eventErr != nil {
			return
		}
		events = append(events, started)
	}
	var offload *toolresult.Record
	if settled {
		item, result, skip, itemErr := toolItem(
			stored,
			existed,
			record.Run.SessionID(),
			runID,
			observation,
		)
		if itemErr != nil || skip {
			return
		}
		stored.Body, err = json.Marshal(item)
		if err != nil {
			return
		}
		offload = result
		completed, eventErr := projector.service.event(
			runID,
			segmentID,
			&facts,
			protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item},
			observation.FinishedAt,
		)
		if eventErr != nil {
			return
		}
		events = append(events, completed)
		if observation.CommittedPlan != nil {
			plan := *observation.CommittedPlan
			if plan.SessionID != record.Run.SessionID() {
				return
			}
			updated, eventErr := projector.service.event(
				runID,
				segmentID,
				&facts,
				protocol.StreamEvent{Type: protocol.StreamPlanUpdated, Plan: &plan},
				observation.FinishedAt,
			)
			if eventErr != nil {
				return
			}
			events = append(events, updated)
		}
	}
	if len(events) == 0 {
		return
	}
	if err := record.Run.Touch(segmentID, projector.service.now().UTC()); err != nil {
		return
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return
	}
	stream, err := newTreeStream(projector.runID, projector.segmentID)
	if err != nil {
		return
	}
	persisted, err := persistEvents(events, facts.EventOrdinal-len(events)+1, stream)
	if err != nil {
		return
	}
	if err := projector.service.store.CommitRunItemEvents(ctx, RunItemEventWrite{
		Run: record, ExpectedSegmentID: segmentID, Item: stored,
		ToolResult: offload, Events: persisted,
	}); err != nil {
		return
	}
	for _, event := range events {
		projector.service.hub.PublishRun(projector.runID, projector.segmentID, event)
	}
}

func transcriptRecord(records []transcript.Record, itemID string) (transcript.Record, bool) {
	for _, record := range records {
		if record.ID == itemID {
			return record, true
		}
	}
	return transcript.Record{}, false
}

func (projector *liveProjector) projectProgress(progress agentexec.ModelProgress) {
	runID, segmentID, ok := projector.target(progress.RunID, progress.SegmentID)
	if !ok {
		return
	}
	state := projector.progress[runID]
	if state == nil {
		state = &liveRunProgress{}
		projector.progress[runID] = state
	}
	mergeLiveUsage(&state.usage, progress.Usage)
	state.steps++
	step := state.steps
	usage := cloneLiveUsage(state.usage)
	value := protocol.RunProgress{Step: &step, Usage: &usage, Activity: "Generating response"}
	if progress.ContextTokens > 0 {
		contextTokens := progress.ContextTokens
		value.ContextTokens = &contextTokens
	}
	if progress.Model != "" {
		value.Activity = "Generating with " + progress.Model
	}
	eventID, err := projector.service.ids.New("evt_")
	if err != nil {
		return
	}
	occurredAt := progress.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = projector.service.now().UTC()
	}
	projector.service.hub.PublishRun(projector.runID, projector.segmentID, protocol.RunEvent{
		RunID: runID, SegmentID: segmentID,
		EventID: eventID, Timestamp: occurredAt,
		Event: protocol.StreamEvent{Type: protocol.StreamSegmentProgress, Progress: &value},
	})
}

func (projector *liveProjector) present(runID string, delta agentexec.ModelDelta) (protocol.Item, protocol.ItemDelta, bool) {
	occurredAt := delta.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = projector.service.now().UTC()
	}
	switch delta.Kind {
	case agentexec.ModelDeltaContent:
		index := delta.Index
		return protocol.Item{
			ID: modelMessageItemID(runID, delta.EffectID), RunID: runID,
			Status: protocol.ItemStatusRunning, CreatedAt: occurredAt, Type: protocol.ItemTypeAgentMessage,
		}, protocol.ItemDelta{Type: protocol.DeltaContent, Index: &index, Text: delta.Text}, true
	case agentexec.ModelDeltaReasoning:
		return protocol.Item{
			ID: modelReasoningItemID(runID, delta.EffectID, delta.Index), RunID: runID,
			Status: protocol.ItemStatusRunning, CreatedAt: occurredAt, Type: protocol.ItemTypeReasoning,
		}, protocol.ItemDelta{Type: protocol.DeltaReasoning, Text: delta.Text}, true
	default:
		return protocol.Item{}, protocol.ItemDelta{}, false
	}
}

func (projector *liveProjector) commitAnchor(runID, segmentID string, item protocol.Item) (protocol.RunEvent, bool) {
	lock := projector.service.runLock(runID)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	record, err := projector.service.store.GetRun(ctx, runID)
	if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != segmentID {
		return protocol.RunEvent{}, false
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	event, err := projector.service.event(
		runID, segmentID, &facts,
		protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}, item.CreatedAt,
	)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	if err := record.Run.Touch(segmentID, projector.service.now().UTC()); err != nil {
		return protocol.RunEvent{}, false
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	stream, err := newTreeStream(projector.runID, projector.segmentID)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal, stream)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	if err := projector.service.store.CommitRunEvent(ctx, RunEventWrite{
		Run: record, ExpectedSegmentID: segmentID, Event: persisted[0],
	}); err != nil {
		return protocol.RunEvent{}, false
	}
	return event, true
}

func (projector *liveProjector) target(runID, segmentID string) (string, string, bool) {
	if runID == "" {
		runID = projector.runID
	}
	if segmentID == "" && runID == projector.runID {
		segmentID = projector.segmentID
	}
	return runID, segmentID, runID != "" && segmentID != ""
}

var _ agentexec.LiveObservationSink = (*liveProjector)(nil)

func cloneLiveUsage(value protocol.Usage) protocol.Usage {
	clone := value
	if value.ByModel != nil {
		clone.ByModel = make(map[string]protocol.ModelUsage, len(value.ByModel))
		for model, usage := range value.ByModel {
			clone.ByModel[model] = usage
		}
	}
	return clone
}

func mergeLiveUsage(total *protocol.Usage, value protocol.Usage) {
	total.InputTokens += value.InputTokens
	total.OutputTokens += value.OutputTokens
	total.CacheReadTokens += value.CacheReadTokens
	total.CacheWriteTokens += value.CacheWriteTokens
	total.ReasoningTokens += value.ReasoningTokens
	mergeUsageCost(&total.CostUSD, value.CostUSD)
	if len(value.ByModel) > 0 && total.ByModel == nil {
		total.ByModel = make(map[string]protocol.ModelUsage)
	}
	for model, usage := range value.ByModel {
		current := total.ByModel[model]
		current.InputTokens += usage.InputTokens
		current.OutputTokens += usage.OutputTokens
		current.CacheReadTokens += usage.CacheReadTokens
		current.CacheWriteTokens += usage.CacheWriteTokens
		current.ReasoningTokens += usage.ReasoningTokens
		mergeUsageCost(&current.CostUSD, usage.CostUSD)
		total.ByModel[model] = current
	}
}
