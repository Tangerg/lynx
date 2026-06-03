// v2 protocol fold — turns each StreamEvent (API.md §5) into an
// AgentViewState mutation. Registered with `host.events.onStream(type, …)`
// by index.ts. Pluginifying these makes the protocol fold a replaceable
// contribution (CLAUDE.md "everything is a plugin").
//
// The view groups contiguous assistant-side Items (agentMessage / reasoning
// / toolCall / question) into one bubble (the "turn"); a userMessage or a
// run boundary opens a fresh turn. Streaming deltas route back to their
// source Item by id (text block `itemId`, reasoning `reasoningId`, tool
// `toolCallId`, toolCalls map key).

import type { Operation } from "fast-json-patch";
import type {
  Interrupt,
  Item,
  ItemDelta,
  OpenInterrupt,
  RunOutcome,
  RunRef,
  StreamEvent,
} from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock, ToolCall } from "@/protocol/run/viewState";
import { applyPatch, deepClone } from "fast-json-patch";
import { appendTimelineEntry, patchRun, setPlan } from "@/plugins/sdk";
import {
  appendToTurn,
  blockStatus,
  contentText,
  formatTime,
  mapPlan,
  mapQuestion,
  nameForRole,
  patchBlock,
  toolFields,
  toolLabel,
  toolStatus,
  updateTool,
  upsertBlock,
} from "./helpers";

const str = (v: unknown): string | undefined => (typeof v === "string" ? v : undefined);

// ---------------------------------------------------------------------------
// run.*
// ---------------------------------------------------------------------------

function onRunStarted(state: AgentViewState, run: RunRef): AgentViewState {
  // Subagent run (spawned by a tool) shares the parent stream but must not
  // reset the main turn / running flag — just record it on the timeline.
  if (run.spawnedByItemId) {
    return appendTimelineEntry({ kind: "run-start", runId: run.id, summary: "subagent" })(state);
  }
  const next: AgentViewState = {
    ...state,
    error: null,
    turnMessageId: null,
    run: { ...state.run, running: true, runId: run.id, threadId: run.sessionId },
  };
  return appendTimelineEntry({ kind: "run-start", runId: run.id })(next);
}

function materializeInterrupt(
  state: AgentViewState,
  it: Interrupt,
  parentRunId: string,
): AgentViewState {
  if (it.kind === "approval") {
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      text: str(it.payload.text) ?? "Approve this action?",
      command: str(it.payload.command) ?? "",
      reason: str(it.payload.reason) ?? "",
      args: (it.payload.arguments ?? it.payload.args) as Record<string, unknown> | undefined,
    };
    const withBlock = appendToTurn(state, it.itemId, block);
    return appendTimelineEntry({
      kind: "approval-request",
      refId: it.itemId,
      summary: block.command,
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
      sessionId: (state.run.threadId ?? "") as OpenInterrupt["sessionId"],
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
    case "userMessage": {
      const msg = {
        id: item.id,
        role: "user" as const,
        who: nameForRole("user"),
        time: formatTime(item.createdAt),
        blocks: [
          {
            kind: "text" as const,
            text: contentText(item.content),
            status: blockStatus(item.status),
          },
        ],
      };
      // A user message opens a fresh assistant turn.
      return { ...state, messages: [...state.messages, msg], turnMessageId: null };
    }
    case "agentMessage":
      return appendToTurn(state, item.id, {
        kind: "text",
        itemId: item.id,
        text: contentText(item.content),
        status: blockStatus(item.status),
      });
    case "reasoning":
      return appendToTurn(state, item.id, {
        kind: "reasoning",
        reasoningId: item.id,
        text: item.text,
        status: blockStatus(item.status),
      });
    case "toolCall": {
      const withBlock = appendToTurn(state, item.id, { kind: "tool", toolCallId: item.id });
      const tool: ToolCall = {
        id: item.id,
        fn: toolLabel(item.tool),
        args: "",
        status: toolStatus(item),
        duration: "LIVE",
        ...toolFields(item.tool),
      };
      const withTool: AgentViewState = {
        ...withBlock,
        toolCalls: { ...withBlock.toolCalls, [item.id]: tool },
      };
      return appendTimelineEntry({ kind: "tool-start", refId: item.id, summary: tool.fn })(
        withTool,
      );
    }
    case "question":
      return appendToTurn(state, item.id, {
        kind: "question",
        status: blockStatus(item.status),
        itemId: item.id,
        questions: mapQuestion(item.question),
      });
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

function onItemCompleted(state: AgentViewState, item: Item): AgentViewState {
  switch (item.type) {
    case "userMessage": {
      if (state.messages.some((m) => m.id === item.id)) return state;
      const msg = {
        id: item.id,
        role: "user" as const,
        who: nameForRole("user"),
        time: formatTime(item.createdAt),
        blocks: [
          { kind: "text" as const, text: contentText(item.content), status: "complete" as const },
        ],
      };
      return { ...state, messages: [...state.messages, msg], turnMessageId: null };
    }
    case "agentMessage":
      return upsertBlock(
        state,
        item.id,
        (b) => b.kind === "text" && b.itemId === item.id,
        () => ({
          kind: "text",
          itemId: item.id,
          text: contentText(item.content),
          status: "complete",
        }),
        (b) =>
          b.kind === "text" ? { ...b, text: contentText(item.content), status: "complete" } : b,
      );
    case "reasoning":
      return upsertBlock(
        state,
        item.id,
        (b) => b.kind === "reasoning" && b.reasoningId === item.id,
        () => ({ kind: "reasoning", reasoningId: item.id, text: item.text, status: "complete" }),
        (b) => (b.kind === "reasoning" ? { ...b, text: item.text, status: "complete" } : b),
      );
    case "toolCall": {
      const fields = toolFields(item.tool);
      const status = toolStatus(item);
      const withBlock =
        state.toolCalls[item.id] === undefined
          ? appendToTurn(state, item.id, { kind: "tool", toolCallId: item.id })
          : state;
      const prev: ToolCall = withBlock.toolCalls[item.id] ?? {
        id: item.id,
        fn: toolLabel(item.tool),
        args: "",
        status,
        duration: "",
      };
      const next: AgentViewState = {
        ...withBlock,
        toolCalls: {
          ...withBlock.toolCalls,
          [item.id]: { ...prev, fn: toolLabel(item.tool), status, duration: "", ...fields },
        },
      };
      return appendTimelineEntry({
        kind: "tool-end",
        refId: item.id,
        status: status === "err" ? "err" : "ok",
        summary: next.toolCalls[item.id]?.fn,
      })(next);
    }
    case "question":
      return patchBlock(
        state,
        (b) => b.kind === "question" && b.itemId === item.id,
        (b) => (b.kind === "question" ? { ...b, status: blockStatus(item.status) } : b),
      );
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
