// v2 protocol fold — turns each StreamEvent (API.md §5) into an
// AgentViewState mutation. Registered with `host.events.onStream(type, …)`
// by index.ts. Pluginifying these makes the protocol fold a replaceable
// contribution (CLAUDE.md "everything is a plugin").
//
// The view groups contiguous assistant-side Items (agentMessage / reasoning
// / toolCall / question) into one bubble (the "turn"); only a userMessage
// opens a fresh turn. Run boundaries do NOT split a turn — a resume after a
// HITL interrupt continues the same bubble — so live streaming groups
// identically to history replay (items.list, which emits no run events).
// Streaming deltas route back to their source Item by id (text block
// `itemId`, reasoning `reasoningId`, tool `toolCallId`, toolCalls map key).

import type { Operation } from "fast-json-patch";
import type {
  Interrupt,
  Item,
  ItemDelta,
  OpenInterrupt,
  RunOutcome,
  RunRef,
  StreamEvent,
  ToolInvocation,
} from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock } from "@/protocol/run/viewState";
import { applyPatch, deepClone } from "fast-json-patch";
import { appendTimelineEntry, patchRun, setPlan } from "@/plugins/sdk";
import { blockStatus, mapPlan, toolLabel } from "./projections";
import {
  appendToTurn,
  appendUserMessage,
  foldQuestion,
  foldReasoning,
  foldText,
  patchBlock,
  updateTool,
  writeToolCall,
} from "./fold";

// Short verb phrase for an approval card title, derived from the tool kind.
function approvalText(tool: ToolInvocation): string {
  switch (tool.kind) {
    case "commandExecution":
      return "Run command";
    case "fileChange":
      return (tool.changes ?? []).length === 1 ? "Apply file change" : "Apply file changes";
    case "search":
      return "Run search";
    case "webSearch":
      return "Run web search";
    case "tool":
      return `Run ${tool.name}`;
  }
}

// ---------------------------------------------------------------------------
// run.*
// ---------------------------------------------------------------------------

function onRunStarted(state: AgentViewState, run: RunRef): AgentViewState {
  // Subagent run (spawned by a tool) shares the parent stream but must not
  // reset the main turn / running flag — just record it on the timeline.
  if (run.spawnedByItemId) {
    return appendTimelineEntry({ kind: "run-start", runId: run.id, summary: "subagent" })(state);
  }
  // run.started does NOT open/close a turn. Turn grouping is driven purely by
  // item-level signals — a userMessage opens a fresh turn (appendUserMessage
  // nulls turnMessageId), assistant Items append to the open one — so LIVE
  // streaming groups bubbles IDENTICALLY to history replay (items.list emits no
  // run events at all). A run boundary is not a turn boundary: a resume/edit
  // continuation after a HITL interrupt is the same logical assistant turn, so
  // its post-approval output stays in the same bubble (no second avatar/name/
  // timestamp). This holds regardless of whether the backend marks the resume
  // RunRef with parentRunId. Only error + run-state are touched here.
  const next: AgentViewState = {
    ...state,
    error: null,
    run: { ...state.run, running: true, runId: run.id, sessionId: run.sessionId },
  };
  return appendTimelineEntry({ kind: "run-start", runId: run.id })(next);
}

function materializeInterrupt(
  state: AgentViewState,
  it: Interrupt,
  parentRunId: string,
): AgentViewState {
  if (it.kind === "approval") {
    // payload.tool is the uniform ToolInvocation (API.md §4.8) — read it
    // directly, no guessing where the command lives / unescaping strings.
    // Tolerate a missing tool (malformed payload) so a buggy backend can't
    // crash the fold and leave an un-actionable interrupt.
    const tool = it.payload.tool as ToolInvocation | undefined;
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      text: tool ? approvalText(tool) : "Approve this action?",
      command: tool?.kind === "commandExecution" ? (tool.command ?? []).join(" ") : "",
      reason: it.payload.reason ?? "",
      // Editable args only make sense for the generic `tool` (its arguments are
      // a free-form object); typed variants carry no editable arg bag.
      args: tool?.kind === "tool" ? tool.arguments : undefined,
      risk: it.payload.risk,
    };
    const withBlock = appendToTurn(state, it.itemId, block);
    return appendTimelineEntry({
      kind: "approval-request",
      refId: it.itemId,
      summary: block.command || toolLabel(tool),
    })(withBlock);
  }
  if (it.kind === "question") {
    // The question Item already produced a question block at item.started —
    // flip it to requires-action + bind the resume target.
    return patchBlock(
      state,
      (b) => b.kind === "question" && b.itemId === it.itemId,
      (b) => ({ ...b, status: "requires-action", parentRunId }),
    );
  }
  return state; // toolResult — gated by features.clientTools, not rendered here
}

function onRunFinished(state: AgentViewState, outcome: RunOutcome): AgentViewState {
  const idle: AgentViewState = { ...state, run: { ...state.run, running: false } };
  if (outcome.type === "interrupt") {
    const open: OpenInterrupt = {
      parentRunId: (state.run.runId ?? "") as OpenInterrupt["parentRunId"],
      sessionId: (state.run.sessionId ?? "") as OpenInterrupt["sessionId"],
      interrupts: outcome.interrupts,
      createdAt: new Date().toISOString(),
    };
    let next: AgentViewState = { ...idle, openInterrupts: [...idle.openInterrupts, open] };
    for (const it of outcome.interrupts) next = materializeInterrupt(next, it, open.parentRunId);
    return next;
  }

  const { result } = outcome;
  const usage = result.usage;
  const tokensUsed = (usage?.inputTokens ?? 0) + (usage?.outputTokens ?? 0);
  const withRun = patchRun({
    running: false,
    step: result.steps ?? state.run.step,
    totalSteps: result.steps ?? state.run.totalSteps,
    tokens: { used: String(tokensUsed), total: state.run.tokens.total },
    cost: result.costUsd !== undefined ? result.costUsd.toFixed(2) : state.run.cost,
  })(idle);

  if (outcome.type === "error") {
    const errored: AgentViewState = {
      ...withRun,
      error: {
        message: result.error?.detail ?? result.error?.type ?? "run failed",
        code: result.error?.type,
      },
    };
    return appendTimelineEntry({
      kind: "run-error",
      status: "err",
      summary: errored.error?.message,
    })(errored);
  }
  return appendTimelineEntry({
    kind: "run-end",
    status: outcome.type === "completed" ? "ok" : undefined,
    summary: outcome.type === "completed" ? undefined : outcome.type,
  })(withRun);
}

// ---------------------------------------------------------------------------
// item.started
// ---------------------------------------------------------------------------

function onItemStarted(state: AgentViewState, item: Item): AgentViewState {
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item, "LIVE");
      return appendTimelineEntry({ kind: "tool-start", refId: item.id, summary: tool.fn })(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setPlan(mapPlan(item.steps))(state);
  }
}

// ---------------------------------------------------------------------------
// item.delta
// ---------------------------------------------------------------------------

function onItemDelta(state: AgentViewState, itemId: string, delta: ItemDelta): AgentViewState {
  switch (delta.type) {
    case "content":
      return patchBlock(
        state,
        (b) => b.kind === "text" && b.itemId === itemId,
        (b) => (b.kind === "text" ? { ...b, text: b.text + delta.text } : b),
      );
    case "reasoning":
      return patchBlock(
        state,
        (b) => b.kind === "reasoning" && b.reasoningId === itemId,
        (b) => (b.kind === "reasoning" ? { ...b, text: b.text + delta.text } : b),
      );
    case "toolArguments":
      return updateTool(state, itemId, (t) => ({ ...t, args: t.args + delta.argumentsTextDelta }));
    case "toolOutput":
      return updateTool(state, itemId, (t) => ({ ...t, result: (t.result ?? "") + delta.text }));
    case "plan":
      return setPlan(mapPlan(delta.steps))(state);
  }
}

// ---------------------------------------------------------------------------
// item.completed
// ---------------------------------------------------------------------------

function onItemCompleted(state: AgentViewState, rawItem: Item): AgentViewState {
  // item.completed ⟹ the item has settled, so its status is terminal
  // (completed | incomplete) — never inProgress. A non-terminal status here
  // means the item was never cleanly finished: history hydration (items.list)
  // of a run lost to a crash/restart still returns its last item as
  // inProgress (the backend reconciles this at the RunRef level, which this
  // item-based fold never reads). Coerce it to incomplete so it renders as a
  // truncated block, not a block that spins forever waiting for a live stream
  // that will never come.
  const item: Item =
    rawItem.status === "inProgress" ? { ...rawItem, status: "incomplete" } : rawItem;
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    // Honor the terminal status: a canceled/interrupted run settles its
    // agentMessage/reasoning as `incomplete` (API.md §4.3), not "complete" —
    // blockStatus maps it so the UI can show the truncated affordance.
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item, "");
      return appendTimelineEntry({
        kind: "tool-end",
        refId: item.id,
        status: tool.status === "err" ? "err" : tool.status === "denied" ? "declined" : "ok",
        summary: tool.fn,
      })(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setPlan(mapPlan(item.steps))(state);
  }
}

// ---------------------------------------------------------------------------
// state.*
// ---------------------------------------------------------------------------

function onStateSnapshot(state: AgentViewState, shared: Record<string, unknown>): AgentViewState {
  return { ...state, shared };
}

function onStateDelta(state: AgentViewState, patch: Operation[]): AgentViewState {
  try {
    const next = applyPatch(deepClone(state.shared), patch, false, false).newDocument;
    return { ...state, shared: next as Record<string, unknown> };
  } catch (err) {
    console.error("[core-reducer] state.delta patch failed:", err);
    return state;
  }
}

// ---------------------------------------------------------------------------
// Registration table — one StreamEventHandler per first-class event type.
// ---------------------------------------------------------------------------

function bind<T extends StreamEvent["type"]>(
  type: T,
  fn: (state: AgentViewState, ev: Extract<StreamEvent, { type: T }>) => AgentViewState,
): [string, StreamEventHandler] {
  return [type, (state, ev) => fn(state, ev as Extract<StreamEvent, { type: T }>)];
}

export const HANDLERS: ReadonlyArray<[string, StreamEventHandler]> = [
  bind("run.started", (s, ev) => onRunStarted(s, ev.run)),
  bind("run.finished", (s, ev) => onRunFinished(s, ev.outcome)),
  bind("item.started", (s, ev) => onItemStarted(s, ev.item)),
  bind("item.delta", (s, ev) => onItemDelta(s, ev.itemId, ev.delta)),
  bind("item.completed", (s, ev) => onItemCompleted(s, ev.item)),
  bind("state.snapshot", (s, ev) => onStateSnapshot(s, ev.state)),
  bind("state.delta", (s, ev) => onStateDelta(s, ev.patch as Operation[])),
];
