import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { PendingInterruptKind } from "@/plugins/sdk/types/agentSessionView";
import type { ApprovalDecision } from "../../domain/hitl";
import type { AgentProblem, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry } from "@/plugins/sdk/types/agentTimeline";
import { selectCurrentRootRun } from "./runTree";
import { isAgentRunFailure } from "./runOutcome";

export interface SettledInterrupt {
  decision?: ApprovalDecision;
  answered?: boolean;
  answers?: string[][];
}

type InterruptBlock = Extract<ContentBlock, { kind: PendingInterruptKind }>;

function matchesInterruptBlock(block: ContentBlock, itemId: string): block is InterruptBlock {
  return (block.kind === "approval" || block.kind === "question") && block.itemId === itemId;
}

function settleInterruptedTool(
  view: AgentSessionView,
  itemId: string,
  status: "denied" | "running",
): AgentSessionView {
  const tool = view.toolCalls[itemId];
  if (!tool || tool.status !== "requires-action") return view;
  return {
    ...view,
    toolCalls: { ...view.toolCalls, [itemId]: { ...tool, status } },
  };
}

/**
 * Reconcile a provisional message identity with its durable identity.
 *
 * The durable message may already have arrived through another projection path
 * before the command acknowledgement names it. In that ordering, retaining the
 * provisional row would render the same user turn twice; collapse it into the
 * existing durable row instead of treating the occupied target as a no-op.
 */
export function reconcileMessageIdentity(
  view: AgentSessionView,
  fromId: string,
  toId: string,
): AgentSessionView {
  if (fromId === toId) return view;
  const has = (id: string) => view.messages.some((message) => message.id === id);
  if (!has(fromId)) return view;
  const targetExists = has(toId);
  return {
    ...view,
    messages: targetExists
      ? view.messages.filter((message) => message.id !== fromId)
      : view.messages.map((message) =>
          message.id === fromId ? { ...message, id: toId } : message,
        ),
    assistantTurnByRunId: Object.fromEntries(
      Object.entries(view.assistantTurnByRunId).map(([runId, messageId]) => [
        runId,
        messageId === fromId ? toId : messageId,
      ]),
    ),
  };
}

export function dropMessage(view: AgentSessionView, id: string): AgentSessionView {
  if (!view.messages.some((message) => message.id === id)) return view;
  return {
    ...view,
    messages: view.messages.filter((message) => message.id !== id),
    assistantTurnByRunId: Object.fromEntries(
      Object.entries(view.assistantTurnByRunId).filter(([, messageId]) => messageId !== id),
    ),
  };
}

export function setCommandError(
  view: AgentSessionView,
  error: AgentProblem | null,
): AgentSessionView {
  if (view.commandError === error) return view;
  return { ...view, commandError: error };
}

export function dismissVisibleProblem(view: AgentSessionView): AgentSessionView {
  const run = selectCurrentRootRun(view);
  const dismissedProblemRunId = isAgentRunFailure(run?.outcome)
    ? run.id
    : view.dismissedProblemRunId;
  if (view.commandError === null && dismissedProblemRunId === view.dismissedProblemRunId) {
    return view;
  }
  return { ...view, commandError: null, dismissedProblemRunId };
}

export function resolveInterrupt(
  view: AgentSessionView,
  itemId: string,
  settled: SettledInterrupt,
  resolvedAt: number,
): AgentSessionView {
  let touchedBlock = false;
  let touchedApproval = false;
  const settledMessages = view.messages.map((message) => {
    if (!message.blocks.some((block) => matchesInterruptBlock(block, itemId))) {
      return message;
    }
    return {
      ...message,
      blocks: message.blocks.map((block) => {
        if (!matchesInterruptBlock(block, itemId)) return block;
        touchedBlock = true;
        if (block.kind === "approval") {
          touchedApproval = true;
          return { ...block, status: "complete" as const, decision: settled.decision };
        }
        return {
          ...block,
          status: "complete" as const,
          answered: settled.answered ?? true,
          answers: settled.answers ?? block.answers,
        };
      }),
    };
  });
  const messages = touchedBlock ? settledMessages : view.messages;

  let touchedInterrupt = false;
  let ownerRunId: string | null = null;
  const settledPendingInterrupts = view.pendingInterrupts.flatMap((group) => {
    const hasItem = group.interrupts.some((interrupt) => interrupt.itemId === itemId);
    if (!hasItem) return [group];
    touchedInterrupt = true;
    ownerRunId = group.runId;
    touchedApproval ||= group.interrupts.some(
      (interrupt) => interrupt.itemId === itemId && interrupt.kind === "approval",
    );
    const interrupts = group.interrupts.filter((interrupt) => interrupt.itemId !== itemId);
    return interrupts.length > 0 ? [{ ...group, interrupts }] : [];
  });
  const pendingInterrupts = touchedInterrupt ? settledPendingInterrupts : view.pendingInterrupts;

  if (!touchedBlock && !touchedInterrupt) return view;

  let next: AgentSessionView = { ...view, messages, pendingInterrupts };
  if (touchedInterrupt) {
    next = settleInterruptedTool(
      next,
      itemId,
      touchedApproval && settled.decision === "declined" ? "denied" : "running",
    );
  }
  if (settled.decision && touchedApproval && ownerRunId) {
    next = appendTimelineEntry({
      id: `timeline:local:approval-result:${itemId}:${settled.decision}`,
      ts: resolvedAt,
      kind: "approval-result",
      runId: ownerRunId,
      refId: itemId,
      status: settled.decision,
    })(next);
  }
  return next;
}
