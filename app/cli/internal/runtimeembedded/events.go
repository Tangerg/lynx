package runtimeembedded

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectEvent(value protocol.RunEvent) (agent.RunEvent, bool, error) {
	projection := runEventProjection{source: value}
	projected, err := projection.project()
	if err != nil || !projected.included {
		return agent.RunEvent{}, projected.included, err
	}
	envelope := projection.envelope(projected.event)
	if err := agent.ValidateEvent(envelope.Event); err != nil {
		return agent.RunEvent{}, false, fmt.Errorf("event %s projection: %w", value.EventID, err)
	}
	return envelope, true, nil
}

type runEventProjection struct {
	source protocol.RunEvent
}

type projectedRunEvent struct {
	event    agent.Event
	included bool
}

func includeRunEvent(event agent.Event) projectedRunEvent {
	return projectedRunEvent{event: event, included: true}
}

func (projection runEventProjection) envelope(event agent.Event) agent.RunEvent {
	return agent.RunEvent{
		EventID: projection.source.EventID,
		RunID:   projection.source.RunID, SegmentID: projection.source.SegmentID,
		At: projection.source.Timestamp, Event: event,
	}
}

func (projection runEventProjection) project() (projectedRunEvent, error) {
	switch projection.source.Event.Type {
	case protocol.StreamSegmentStarted:
		return projection.segmentStarted()
	case protocol.StreamItemStarted:
		return projection.itemStarted()
	case protocol.StreamItemDelta:
		return projection.itemDelta()
	case protocol.StreamItemCompleted:
		return projection.itemCompleted()
	case protocol.StreamStateSnapshot:
		return projection.stateSnapshot()
	case protocol.StreamSegmentFinished:
		return projection.segmentFinished()
	case protocol.StreamSegmentProgress:
		return projection.segmentProgress()
	case protocol.StreamCustom:
		return projection.custom()
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s has unsupported authoritative type %q", projection.source.EventID, projection.source.Event.Type)
	}
}

func (projection runEventProjection) segmentProgress() (projectedRunEvent, error) {
	value := projection.source.Event.Progress
	if value == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.progress has no progress", projection.source.EventID)
	}
	progress := agent.RunProgress{
		Activity: value.Activity,
	}
	if value.Step != nil {
		progress.Step = new(*value.Step)
	}
	if value.ContextTokens != nil {
		progress.ContextTokens = new(*value.ContextTokens)
	}
	if value.Usage != nil {
		usage := projectModelUsage(*value.Usage)
		progress.Usage = &usage
	}
	return includeRunEvent(progress), nil
}

func (projection runEventProjection) custom() (projectedRunEvent, error) {
	payload, err := json.Marshal(projection.source.Event.Payload)
	if err != nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: encode custom payload: %w", projection.source.EventID, err)
	}
	return includeRunEvent(agent.CustomEvent{Name: projection.source.Event.Name, PayloadJSON: payload}), nil
}

func (projection runEventProjection) segmentStarted() (projectedRunEvent, error) {
	if projection.source.Event.Run == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.started has no run", projection.source.EventID)
	}
	run, err := projectRun(*projection.source.Event.Run)
	return includeRunEvent(agent.SegmentStarted{Run: run}), err
}

func (projection runEventProjection) itemStarted() (projectedRunEvent, error) {
	if projection.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.started has no item", projection.source.EventID)
	}
	block, err := projectItem(*projection.source.Event.Item)
	return includeRunEvent(agent.BlockStarted{Block: block}), err
}

func (projection runEventProjection) itemCompleted() (projectedRunEvent, error) {
	if projection.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.completed has no item", projection.source.EventID)
	}
	block, err := projectItem(*projection.source.Event.Item)
	return includeRunEvent(agent.BlockCompleted{Block: block}), err
}

func (projection runEventProjection) itemDelta() (projectedRunEvent, error) {
	delta := projection.source.Event.Delta
	if delta == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.delta has no delta", projection.source.EventID)
	}
	switch delta.Type {
	case protocol.DeltaToolArguments:
		return includeRunEvent(agent.ToolArgumentsDelta{
			BlockID: projection.source.Event.ItemID, Text: delta.ArgumentsTextDelta,
		}), nil
	case protocol.DeltaContent, protocol.DeltaReasoning, protocol.DeltaToolOutput:
		return includeRunEvent(agent.BlockDelta{BlockID: projection.source.Event.ItemID, Text: delta.Text}), nil
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s: unsupported item delta %q", projection.source.EventID, delta.Type)
	}
}

func (projection runEventProjection) stateSnapshot() (projectedRunEvent, error) {
	items, revision, err := projectPlan(projection.source.Event.State)
	if err != nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: %w", projection.source.EventID, err)
	}
	return includeRunEvent(agent.PlanChanged{Revision: revision, Items: items}), nil
}

func (projection runEventProjection) segmentFinished() (projectedRunEvent, error) {
	stream := projection.source.Event
	if stream.Outcome == nil || stream.Metrics == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.finished is incomplete", projection.source.EventID)
	}
	usage := projectUsage(*stream.Metrics)
	switch stream.Outcome.Type {
	case protocol.SegmentInterrupt:
		interactions, err := projectInteractions(stream.Outcome.Interrupts)
		if err != nil {
			return projectedRunEvent{}, fmt.Errorf("event %s: %w", projection.source.EventID, err)
		}
		return includeRunEvent(agent.RunInterrupted{Interactions: interactions, Usage: usage}), nil
	case protocol.SegmentSuspended:
		return includeRunEvent(agent.RunSuspended{Usage: usage}), nil
	default:
		return includeRunEvent(agent.RunFinished{Outcome: projectOutcome(*stream.Outcome), Usage: usage}), nil
	}
}
