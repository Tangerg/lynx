package runtimeembedded

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func projectEvent(value protocol.RunEvent) (agent.RunEvent, bool, error) {
	envelope := agent.RunEvent{
		EventID: value.EventID, RunID: value.RunID, SegmentID: value.SegmentID, At: value.Timestamp,
	}
	switch value.Event.Type {
	case protocol.StreamSegmentStarted:
		if value.Event.Run == nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: segment.started has no run", value.EventID)
		}
		run, err := projectRun(*value.Event.Run)
		if err != nil {
			return agent.RunEvent{}, false, err
		}
		envelope.Event = agent.SegmentStarted{Run: run}
	case protocol.StreamItemStarted:
		if value.Event.Item == nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: item.started has no item", value.EventID)
		}
		block, err := projectItem(*value.Event.Item)
		if err != nil {
			return agent.RunEvent{}, false, err
		}
		envelope.Event = agent.BlockStarted{Block: block}
	case protocol.StreamItemDelta:
		if value.Event.Delta == nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: item.delta has no delta", value.EventID)
		}
		if value.Event.Delta.Type == protocol.DeltaToolArguments {
			return agent.RunEvent{}, false, nil
		}
		switch value.Event.Delta.Type {
		case protocol.DeltaContent, protocol.DeltaReasoning, protocol.DeltaToolOutput:
			envelope.Event = agent.BlockDelta{BlockID: value.Event.ItemID, Text: value.Event.Delta.Text}
		default:
			return agent.RunEvent{}, false, fmt.Errorf("event %s: unsupported item delta %q", value.EventID, value.Event.Delta.Type)
		}
	case protocol.StreamItemCompleted:
		if value.Event.Item == nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: item.completed has no item", value.EventID)
		}
		block, err := projectItem(*value.Event.Item)
		if err != nil {
			return agent.RunEvent{}, false, err
		}
		envelope.Event = agent.BlockCompleted{Block: block}
	case protocol.StreamStateSnapshot:
		items, revision, err := projectPlan(value.Event.State)
		if err != nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: %w", value.EventID, err)
		}
		envelope.Event = agent.PlanChanged{Revision: revision, Items: items}
	case protocol.StreamSegmentFinished:
		if value.Event.Outcome == nil || value.Event.Metrics == nil {
			return agent.RunEvent{}, false, fmt.Errorf("event %s: segment.finished is incomplete", value.EventID)
		}
		usage := projectUsage(*value.Event.Metrics)
		switch value.Event.Outcome.Type {
		case protocol.SegmentInterrupt:
			interactions, err := projectInteractions(value.Event.Outcome.Interrupts)
			if err != nil {
				return agent.RunEvent{}, false, fmt.Errorf("event %s: %w", value.EventID, err)
			}
			envelope.Event = agent.RunInterrupted{Interactions: interactions, Usage: usage}
		case protocol.SegmentSuspended:
			return agent.RunEvent{}, false, fmt.Errorf("%w: event %s suspended for a child run", agent.ErrIncompatibleRuntime, value.EventID)
		default:
			envelope.Event = agent.RunFinished{Outcome: projectOutcome(*value.Event.Outcome), Usage: usage}
		}
	case protocol.StreamSegmentProgress, protocol.StreamCustom:
		return agent.RunEvent{}, false, nil
	default:
		return agent.RunEvent{}, false, fmt.Errorf("event %s has unsupported authoritative type %q", value.EventID, value.Event.Type)
	}
	if err := agent.ValidateEvent(envelope.Event); err != nil {
		return agent.RunEvent{}, false, fmt.Errorf("event %s projection: %w", value.EventID, err)
	}
	return envelope, true, nil
}
