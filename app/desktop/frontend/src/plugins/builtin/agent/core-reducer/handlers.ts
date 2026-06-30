import type { Operation } from "fast-json-patch";
import type { StreamEvent } from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentViewState } from "@/protocol/run/viewState";
import { onItemCompleted, onItemDelta, onItemStarted } from "./itemHandlers";
import { onRunFinished, onRunProgress, onRunStarted } from "./runHandlers";
import { onStateDelta, onStateSnapshot } from "./stateHandlers";

function bind<T extends StreamEvent["type"]>(
  type: T,
  fn: (
    state: AgentViewState,
    ev: Extract<StreamEvent, { type: T }>,
    runId?: string,
  ) => AgentViewState,
): [string, StreamEventHandler] {
  return [type, (state, ev, runId) => fn(state, ev as Extract<StreamEvent, { type: T }>, runId)];
}

export const HANDLERS: ReadonlyArray<[string, StreamEventHandler]> = [
  bind("run.started", (state, event) => onRunStarted(state, event.run)),
  bind("run.progress", (state, event, runId) => onRunProgress(state, event.progress, runId)),
  bind("run.finished", (state, event, runId) => onRunFinished(state, event.outcome, runId)),
  bind("item.started", (state, event) => onItemStarted(state, event.item)),
  bind("item.delta", (state, event) => onItemDelta(state, event.itemId, event.delta)),
  bind("item.completed", (state, event) => onItemCompleted(state, event.item)),
  bind("state.snapshot", (state, event) => onStateSnapshot(state, event.state)),
  bind("state.delta", (state, event) => onStateDelta(state, event.patch as Operation[])),
];
