import type { StreamEvent } from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";
import { onItemCompleted, onItemDelta, onItemStarted } from "./itemHandlers";
import { onRunFinished, onRunProgress, onRunStarted } from "./runHandlers";
import { onStateSnapshot } from "./stateHandlers";

function bind<T extends StreamEvent["type"]>(
  type: T,
  fn: (
    state: AgentViewState,
    ev: Extract<StreamEvent, { type: T }>,
    runId?: string,
    segmentId?: string,
  ) => AgentViewState,
): [string, StreamEventHandler] {
  return [
    type,
    (state, ev, runId, segmentId) =>
      fn(state, ev as Extract<StreamEvent, { type: T }>, runId, segmentId),
  ];
}

export const HANDLERS: ReadonlyArray<[string, StreamEventHandler]> = [
  bind("segment.started", (state, event, _runId, segmentId) =>
    onRunStarted(state, event.run, segmentId),
  ),
  bind("segment.progress", (state, event, runId) => onRunProgress(state, event.progress, runId)),
  bind("segment.finished", (state, event, runId) => onRunFinished(state, event.outcome, runId)),
  bind("item.started", (state, event) => onItemStarted(state, event.item)),
  bind("item.delta", (state, event) => onItemDelta(state, event.itemId, event.delta)),
  bind("item.completed", (state, event) => onItemCompleted(state, event.item)),
  bind("state.snapshot", (state, event) => onStateSnapshot(state, event.state)),
];
