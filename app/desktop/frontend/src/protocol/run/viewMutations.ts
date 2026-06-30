import type { AgentViewState, RunError } from "./viewState";
import { appendTimelineEntry } from "./timeline";

export interface SettledInterrupt {
  decision?: "approved" | "declined";
  answered?: boolean;
  answers?: Record<string, string[]>;
}

export function relabelMessage(view: AgentViewState, fromId: string, toId: string): AgentViewState {
  if (fromId === toId) return view;
  const has = (id: string) => view.messages.some((m) => m.id === id);
  if (!has(fromId) || has(toId)) return view;
  return {
    ...view,
    messages: view.messages.map((m) => (m.id === fromId ? { ...m, id: toId } : m)),
  };
}

export function dropMessage(view: AgentViewState, id: string): AgentViewState {
  if (!view.messages.some((m) => m.id === id)) return view;
  return { ...view, messages: view.messages.filter((m) => m.id !== id) };
}

export function setRunError(view: AgentViewState, error: RunError | null): AgentViewState {
  if (view.error === error) return view;
  return { ...view, error };
}

export function cancelRunningRun(view: AgentViewState): AgentViewState {
  if (!view.run.running) return view;
  return appendTimelineEntry({ kind: "run-end", status: undefined, summary: "canceled" })({
    ...view,
    run: { ...view.run, running: false },
  });
}

export function resolveInterrupt(
  view: AgentViewState,
  itemId: string,
  settled: SettledInterrupt,
): AgentViewState {
  const messages = view.messages.map((m) => {
    if (!m.blocks.some((b) => "itemId" in b && b.itemId === itemId)) return m;
    return {
      ...m,
      blocks: m.blocks.map((b) => {
        if (!("itemId" in b) || b.itemId !== itemId) return b;
        if (b.kind === "approval")
          return { ...b, status: "complete" as const, decision: settled.decision };
        if (b.kind === "question")
          return {
            ...b,
            status: "complete" as const,
            answered: settled.answered ?? true,
            answers: settled.answers ?? b.answers,
          };
        return b;
      }),
    };
  });
  const openInterrupts = view.openInterrupts
    .map((oi) => ({ ...oi, interrupts: oi.interrupts.filter((i) => i.itemId !== itemId) }))
    .filter((oi) => oi.interrupts.length > 0);
  let next: AgentViewState = { ...view, messages, openInterrupts };
  if (settled.decision) {
    next = appendTimelineEntry({
      kind: "approval-result",
      refId: itemId,
      status: settled.decision,
    })(next);
  }
  return next;
}
