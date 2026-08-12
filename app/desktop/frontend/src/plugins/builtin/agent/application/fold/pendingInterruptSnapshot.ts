import type { AgentPendingInterruptSet } from "@/plugins/sdk";
import type { AgentSessionView, PendingInterrupt } from "@/plugins/sdk/types/agentSessionView";
import { materializeInterrupt } from "./interruptMaterialization";

export function foldPendingInterruptSet(
  state: AgentSessionView,
  snapshot: AgentPendingInterruptSet,
): AgentSessionView {
  let next = state;
  const byRunId = new Map<string, PendingInterrupt[]>();
  for (const interrupt of snapshot.interrupts) {
    const current = byRunId.get(interrupt.runId) ?? [];
    current.push({ itemId: interrupt.itemId, kind: interrupt.type });
    byRunId.set(interrupt.runId, current);
  }

  for (const [runId, interrupts] of byRunId) {
    next = mergeGroup(next, snapshot.sessionId, runId, snapshot.rootRunId, interrupts);
  }
  for (const interrupt of snapshot.interrupts) {
    next = materializeInterrupt(
      next,
      interrupt,
      {
        runId: interrupt.runId,
        segmentId: null,
        eventId: `snapshot:${snapshot.rootRunId}:interrupt:${interrupt.itemId}`,
        timestamp: snapshot.createdAt,
      },
      snapshot.rootRunId,
    );
  }
  return next;
}

function mergeGroup(
  state: AgentSessionView,
  sessionId: string,
  runId: string,
  rootRunId: string,
  interrupts: PendingInterrupt[],
): AgentSessionView {
  const existing = state.pendingInterrupts.find(
    (group) => group.runId === runId && group.rootRunId === rootRunId,
  );
  const known = new Set(existing?.interrupts.map((interrupt) => interrupt.itemId));
  const fresh = interrupts.filter((interrupt) => !known.has(interrupt.itemId));
  if (fresh.length === 0) return state;
  if (!existing) {
    return {
      ...state,
      pendingInterrupts: [
        ...state.pendingInterrupts,
        { runId, rootRunId, sessionId, interrupts: fresh },
      ],
    };
  }
  return {
    ...state,
    pendingInterrupts: state.pendingInterrupts.map((group) =>
      group.runId === runId && group.rootRunId === rootRunId
        ? { ...group, interrupts: [...group.interrupts, ...fresh] }
        : group,
    ),
  };
}
