package runflow

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
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

type liveProjector struct {
	service   *Service
	runID     string
	segmentID string
	input     chan agentexec.ModelDelta
	done      chan struct{}
	started   map[string]bool
}

func newLiveProjector(service *Service, runID, segmentID string) *liveProjector {
	projector := &liveProjector{
		service: service, runID: runID, segmentID: segmentID,
		input: make(chan agentexec.ModelDelta, liveProjectionCapacity),
		done: make(chan struct{}), started: make(map[string]bool),
	}
	go projector.run()
	return projector
}

// OfferModelDelta is deliberately non-blocking. Provider streaming is never
// allowed to wait on SQLite or a downstream renderer.
func (projector *liveProjector) OfferModelDelta(delta agentexec.ModelDelta) {
	select {
	case projector.input <- delta:
	default:
	}
}

func (projector *liveProjector) Close() {
	close(projector.input)
	<-projector.done
}

func (projector *liveProjector) run() {
	defer close(projector.done)
	for delta := range projector.input {
		projector.project(delta)
	}
}

func (projector *liveProjector) project(delta agentexec.ModelDelta) {
	if delta.EffectID == "" || delta.Text == "" || delta.Index < 0 {
		return
	}
	item, itemDelta, ok := projector.present(delta)
	if !ok {
		return
	}
	if !projector.started[item.ID] {
		started, committed := projector.commitAnchor(item)
		if !committed {
			return
		}
		projector.started[item.ID] = true
		projector.service.hub.PublishRun(started)
	}
	eventID, err := projector.service.ids.New("evt_")
	if err != nil {
		return
	}
	occurredAt := delta.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = projector.service.now().UTC()
	}
	projector.service.hub.PublishRun(protocol.RunEvent{
		RunID: projector.runID, SegmentID: projector.segmentID,
		EventID: eventID, Timestamp: occurredAt,
		Event: protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: item.ID, Delta: &itemDelta},
	})
}

func (projector *liveProjector) present(delta agentexec.ModelDelta) (protocol.Item, protocol.ItemDelta, bool) {
	occurredAt := delta.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = projector.service.now().UTC()
	}
	switch delta.Kind {
	case agentexec.ModelDeltaContent:
		index := delta.Index
		return protocol.Item{
			ID: modelMessageItemID(projector.runID, delta.EffectID), RunID: projector.runID,
			Status: protocol.ItemStatusRunning, CreatedAt: occurredAt, Type: protocol.ItemTypeAgentMessage,
		}, protocol.ItemDelta{Type: protocol.DeltaContent, Index: &index, Text: delta.Text}, true
	case agentexec.ModelDeltaReasoning:
		return protocol.Item{
			ID: modelReasoningItemID(projector.runID, delta.EffectID, delta.Index), RunID: projector.runID,
			Status: protocol.ItemStatusRunning, CreatedAt: occurredAt, Type: protocol.ItemTypeReasoning,
		}, protocol.ItemDelta{Type: protocol.DeltaReasoning, Text: delta.Text}, true
	default:
		return protocol.Item{}, protocol.ItemDelta{}, false
	}
}

func (projector *liveProjector) commitAnchor(item protocol.Item) (protocol.RunEvent, bool) {
	lock := projector.service.runLock(projector.runID)
	lock.Lock()
	defer lock.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	record, err := projector.service.store.GetRun(ctx, projector.runID)
	if err != nil || record.Run.Status() != rundomain.Running || record.Run.ActiveSegmentID() != projector.segmentID {
		return protocol.RunEvent{}, false
	}
	facts, err := decodeFacts(record.Body)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	event, err := projector.service.event(
		projector.runID, projector.segmentID, &facts,
		protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}, item.CreatedAt,
	)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	if err := record.Run.Touch(projector.segmentID, projector.service.now().UTC()); err != nil {
		return protocol.RunEvent{}, false
	}
	record, err = makeRecord(record.Run, facts)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	persisted, err := persistEvents([]protocol.RunEvent{event}, facts.EventOrdinal)
	if err != nil {
		return protocol.RunEvent{}, false
	}
	if err := projector.service.store.CommitRunEvent(ctx, RunEventWrite{
		Run: record, ExpectedSegmentID: projector.segmentID, Event: persisted[0],
	}); err != nil {
		return protocol.RunEvent{}, false
	}
	return event, true
}

var _ agentexec.ModelDeltaSink = (*liveProjector)(nil)
