package runtimeembedded

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectEvent(value protocol.RunEvent) (agent.RunEvent, bool, error) {
	if value.Timestamp.IsZero() {
		return agent.RunEvent{}, false, fmt.Errorf("event %s timestamp is zero", value.EventID)
	}
	projection := runEventProjection{source: value}
	projected, err := projection.project()
	if err != nil || !projected.included {
		return agent.RunEvent{}, projected.included, err
	}
	envelope := projection.envelope(projected.event)
	if err := envelope.Validate(); err != nil {
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

func (r runEventProjection) envelope(event agent.Event) agent.RunEvent {
	return agent.RunEvent{
		EventID: r.source.EventID,
		RunID:   r.source.RunID, SegmentID: r.source.SegmentID,
		At: r.source.Timestamp, Event: event,
	}
}

func (r runEventProjection) project() (projectedRunEvent, error) {
	switch r.source.Event.Type {
	case protocol.StreamSegmentStarted:
		return r.segmentStarted()
	case protocol.StreamItemStarted:
		return r.itemStarted()
	case protocol.StreamItemDelta:
		return r.itemDelta()
	case protocol.StreamItemCompleted:
		return r.itemCompleted()
	case protocol.StreamPlanUpdated:
		return r.planUpdated()
	case protocol.StreamSegmentFinished:
		return r.segmentFinished()
	case protocol.StreamSegmentProgress:
		return r.segmentProgress()
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s has unsupported authoritative type %q", r.source.EventID, r.source.Event.Type)
	}
}

func (r runEventProjection) segmentProgress() (projectedRunEvent, error) {
	value := r.source.Event.Progress
	if value == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.progress has no progress", r.source.EventID)
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
		usage := projectUsageBreakdown(*value.Usage)
		progress.Usage = &usage
	}
	return includeRunEvent(progress), nil
}

func (r runEventProjection) segmentStarted() (projectedRunEvent, error) {
	if r.source.Event.Run == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.started has no run", r.source.EventID)
	}
	run, err := projectRun(*r.source.Event.Run)
	return includeRunEvent(agent.SegmentStarted{Run: run}), err
}

func (r runEventProjection) itemStarted() (projectedRunEvent, error) {
	if r.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.started has no item", r.source.EventID)
	}
	block, err := projectItem(*r.source.Event.Item)
	return includeRunEvent(agent.BlockStarted{Block: block}), err
}

func (r runEventProjection) itemCompleted() (projectedRunEvent, error) {
	if r.source.Event.Item == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.completed has no item", r.source.EventID)
	}
	block, err := projectItem(*r.source.Event.Item)
	return includeRunEvent(agent.BlockCompleted{Block: block}), err
}

func (r runEventProjection) itemDelta() (projectedRunEvent, error) {
	delta := r.source.Event.Delta
	if delta == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: item.delta has no delta", r.source.EventID)
	}
	switch delta.Type {
	case protocol.DeltaToolArguments:
		return includeRunEvent(agent.ToolArgumentsDelta{
			BlockID: r.source.Event.ItemID, Text: delta.ArgumentsTextDelta,
		}), nil
	case protocol.DeltaContent:
		projected := agent.BlockDelta{BlockID: r.source.Event.ItemID, Text: delta.Text}
		if delta.Index != nil {
			projected.ContentIndex = new(*delta.Index)
		}
		return includeRunEvent(projected), nil
	case protocol.DeltaReasoning, protocol.DeltaToolOutput:
		return includeRunEvent(agent.BlockDelta{BlockID: r.source.Event.ItemID, Text: delta.Text}), nil
	default:
		return projectedRunEvent{}, fmt.Errorf("event %s: unsupported item delta %q", r.source.EventID, delta.Type)
	}
}

func (r runEventProjection) planUpdated() (projectedRunEvent, error) {
	items, revision, err := projectPlan(r.source.Event.Plan)
	if err != nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: %w", r.source.EventID, err)
	}
	return includeRunEvent(agent.PlanChanged{Revision: revision, Items: items}), nil
}

func (r runEventProjection) segmentFinished() (projectedRunEvent, error) {
	stream := r.source.Event
	if stream.Outcome == nil || stream.Metrics == nil {
		return projectedRunEvent{}, fmt.Errorf("event %s: segment.finished is incomplete", r.source.EventID)
	}
	usage := projectUsage(*stream.Metrics)
	switch stream.Outcome.Type {
	case protocol.SegmentInterrupt:
		interactions, err := projectInteractions(stream.Outcome.Interrupts)
		if err != nil {
			return projectedRunEvent{}, fmt.Errorf("event %s: %w", r.source.EventID, err)
		}
		return includeRunEvent(agent.RunInterrupted{Interactions: interactions, Usage: usage}), nil
	case protocol.SegmentSuspended:
		return includeRunEvent(agent.RunSuspended{Usage: usage}), nil
	default:
		outcome, err := projectOutcome(*stream.Outcome)
		if err != nil {
			return projectedRunEvent{}, fmt.Errorf("event %s: %w", r.source.EventID, err)
		}
		return includeRunEvent(agent.RunFinished{Outcome: outcome, Usage: usage}), nil
	}
}
