import type { RunEvent, StreamEvent } from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { onItemCompleted, onItemDelta, onItemStarted } from "./itemHandlers";
import { onRunFinished, onRunProgress, onRunStarted } from "./runHandlers";
import { runEventSource } from "./source";
import { onStateSnapshot } from "./stateHandlers";

function bind<T extends StreamEvent["type"]>(
  type: T,
  fold: (
    state: AgentSessionView,
    event: Extract<StreamEvent, { type: T }>,
    envelope: RunEvent,
  ) => AgentSessionView,
): [string, StreamEventHandler] {
  return [
    type,
    (state, envelope) => fold(state, envelope.event as Extract<StreamEvent, { type: T }>, envelope),
  ];
}

export const HANDLERS: ReadonlyArray<[string, StreamEventHandler]> = [
  bind("segment.started", (state, event, envelope) =>
    onRunStarted(state, event.run, runEventSource(envelope)),
  ),
  bind("segment.progress", (state, event, envelope) =>
    onRunProgress(state, event.progress, runEventSource(envelope)),
  ),
  bind("segment.finished", (state, event, envelope) =>
    onRunFinished(state, event.outcome, event.metrics, runEventSource(envelope)),
  ),
  bind("item.started", (state, event, envelope) =>
    onItemStarted(state, event.item, runEventSource(envelope)),
  ),
  bind("item.delta", (state, event, envelope) =>
    onItemDelta(state, event.itemId, event.delta, runEventSource(envelope)),
  ),
  bind("item.completed", (state, event, envelope) =>
    onItemCompleted(state, event.item, runEventSource(envelope)),
  ),
  bind("state.snapshot", (state, event) => onStateSnapshot(state, event.state)),
];
