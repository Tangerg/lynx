import type { AgentEventEnvelope, AgentStreamEvent, StreamEventHandler } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { onItemCompleted, onItemDelta, onItemStarted } from "../application/fold/itemHandlers";
import { onRunFinished, onRunProgress, onRunStarted } from "../application/fold/runHandlers";
import { runEventSource } from "../application/fold/source";
import { onPlanUpdated } from "../application/fold/planHandlers";

function bind<T extends AgentStreamEvent["type"]>(
  type: T,
  fold: (
    state: AgentSessionView,
    event: Extract<AgentStreamEvent, { type: T }>,
    envelope: AgentEventEnvelope,
  ) => AgentSessionView,
): [string, StreamEventHandler] {
  return [
    type,
    (state, envelope) =>
      fold(state, envelope.event as Extract<AgentStreamEvent, { type: T }>, envelope),
  ];
}

/** Runtime event dispatch lives at the Adapter boundary. The wire Plan is
 * translated here before it enters the Agent product fold. */
export const RUNTIME_EVENT_HANDLERS: ReadonlyArray<[string, StreamEventHandler]> = [
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
  bind("plan.updated", (state, event) => onPlanUpdated(state, event.plan)),
];
