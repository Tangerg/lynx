import type {
  AgentViewState,
  ContentBlock,
  RunError,
} from "@/plugins/builtin/agent/public/viewState";
import { appendTimelineEntry } from "@/plugins/builtin/agent/public/viewState";

export interface SettledInterrupt {
  decision?: "approved" | "declined";
  answered?: boolean;
  answers?: Record<string, string[]>;
}

type InterruptBlock = Extract<ContentBlock, { kind: "approval" | "question" }>;

function matchesInterruptBlock(block: ContentBlock, itemId: string): block is InterruptBlock {
  return (block.kind === "approval" || block.kind === "question") && block.itemId === itemId;
}

function settleInterruptedTool(
  view: AgentViewState,
  itemId: string,
  status: "denied" | "running",
): AgentViewState {
  const tool = view.toolCalls[itemId];
  if (!tool || tool.status !== "requires-action") return view;
  return {
    ...view,
    toolCalls: { ...view.toolCalls, [itemId]: { ...tool, status } },
  };
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
  let touchedBlock = false;
  let touchedApproval = false;
  const settledMessages = view.messages.map((m) => {
    if (!m.blocks.some((b) => matchesInterruptBlock(b, itemId))) return m;
    return {
      ...m,
      blocks: m.blocks.map((b) => {
        if (!matchesInterruptBlock(b, itemId)) return b;
        touchedBlock = true;
        if (b.kind === "approval") {
          touchedApproval = true;
          return { ...b, status: "complete" as const, decision: settled.decision };
        }
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
  const messages = touchedBlock ? settledMessages : view.messages;

  let touchedInterrupt = false;
  const settledOpenInterrupts = view.openInterrupts.flatMap((oi) => {
    const hasItem = oi.interrupts.some((i) => i.itemId === itemId);
    if (!hasItem) return [oi];
    touchedInterrupt = true;
    touchedApproval ||= oi.interrupts.some((i) => i.itemId === itemId && i.type === "approval");
    const interrupts = oi.interrupts.filter((i) => i.itemId !== itemId);
    return interrupts.length > 0 ? [{ ...oi, interrupts }] : [];
  });
  const openInterrupts = touchedInterrupt ? settledOpenInterrupts : view.openInterrupts;

  if (!touchedBlock && !touchedInterrupt) return view;

  let next: AgentViewState = { ...view, messages, openInterrupts };
  if (touchedInterrupt) {
    next = settleInterruptedTool(
      next,
      itemId,
      touchedApproval && settled.decision === "declined" ? "denied" : "running",
    );
  }
  if (settled.decision && touchedApproval) {
    next = appendTimelineEntry({
      kind: "approval-result",
      refId: itemId,
      status: settled.decision,
    })(next);
  }
  return next;
}
