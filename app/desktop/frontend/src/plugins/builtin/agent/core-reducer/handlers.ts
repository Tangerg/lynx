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
  RunProgress,
  RunRef,
  StreamEvent,
  ToolInvocation,
} from "@/rpc";
import type { StreamEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock } from "@/protocol/run/viewState";
import { applyPatch, deepClone } from "fast-json-patch";
import { appendTimelineEntry, patchRun, setPlan } from "@/plugins/sdk";
import {
  approvalText,
  blockStatus,
  commandString,
  editableArgs,
  mapPlan,
  mapQuestion,
  toolLabel,
} from "./projections";
import {
  appendToTurn,
  appendUserMessage,
  foldQuestion,
  foldReasoning,
  foldText,
  patchBlock,
  settleOpenInterrupts,
  updateTool,
  writeToolCall,
} from "./fold";

// run.*

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

// Live progress preview (ephemeral). Mirrors the run.finished mapping but only
// patches the fields the event carries — the authoritative totals still settle
// on run.finished (§5.2). Subagent progress (no way to tell here) harmlessly
// updates the same readout.
function onRunProgress(state: AgentViewState, progress: RunProgress): AgentViewState {
  const usage = progress.usage;
  const tokensUsed =
    usage !== undefined ? (usage.inputTokens ?? 0) + (usage.outputTokens ?? 0) : undefined;
  // Cost reads usage.costUsd — there is no separate RunProgress.costUsd (§5).
  const costUsd = usage?.costUsd;
  return patchRun({
    ...(progress.step !== undefined ? { step: progress.step } : {}),
    ...(progress.maxSteps !== undefined ? { totalSteps: progress.maxSteps } : {}),
    ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
    ...(tokensUsed !== undefined
      ? { tokens: { used: String(tokensUsed), total: state.run.tokens.total } }
      : {}),
    ...(costUsd !== undefined ? { cost: costUsd.toFixed(2) } : {}),
  })(state);
}

function materializeInterrupt(
  state: AgentViewState,
  it: Interrupt,
  parentRunId: string,
): AgentViewState {
  if (it.type === "approval") {
    // payload.tool is the uniform ToolInvocation (API.md §4.8) — name+arguments
    // are always present, no guessing where the command lives. Tolerate a
    // missing tool (malformed payload) so a buggy backend can't crash the fold
    // and leave an un-actionable interrupt.
    const tool = it.payload?.tool as ToolInvocation | undefined;
    // Upsert — mirror the question branch below. A re-delivered interrupt
    // (reconnect / replay re-seeing the same run.finished) must re-affirm the
    // existing card, not append a second approval block with the same itemId
    // (→ React duplicate-key warning + two cards).
    if (
      state.messages.some((m) =>
        m.blocks.some((b) => b.kind === "approval" && b.itemId === it.itemId),
      )
    ) {
      return patchBlock(
        state,
        (b) => b.kind === "approval" && b.itemId === it.itemId,
        (b) => ({ ...b, status: "requires-action", parentRunId }),
      );
    }
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      text: tool ? approvalText(tool) : "Approve this action?",
      command: tool ? commandString(tool) : "",
      reason: it.payload?.reason ?? "",
      args: tool ? editableArgs(tool) : undefined,
      risk: it.payload?.risk,
    };
    const withBlock = appendToTurn(state, it.itemId, block);
    return appendTimelineEntry({
      kind: "approval-request",
      refId: it.itemId,
      summary: block.command || toolLabel(tool),
    })(withBlock);
  }
  if (it.type === "question") {
    // The interrupt payload is self-contained (API.md §4.8, S1): it carries the
    // Question, so the card materializes from the payload even if item.started
    // was missed (e.g. process restart while the question was still running and
    // not yet in durable history). Upsert: patch the block in place if the
    // item.started already produced it, else create it from the payload.
    const hasBlock = state.messages.some((m) =>
      m.blocks.some((b) => b.kind === "question" && b.itemId === it.itemId),
    );
    if (hasBlock) {
      return patchBlock(
        state,
        (b) => b.kind === "question" && b.itemId === it.itemId,
        (b) => ({ ...b, status: "requires-action", parentRunId }),
      );
    }
    return appendToTurn(state, it.itemId, {
      kind: "question",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      questions: mapQuestion(it.payload?.question),
    });
  }
  return state; // toolResult — gated by features.clientTools, not rendered here
}

// A subagent run shares the parent stream (API.md §5.4). RunOutcome carries no
// id, so we discriminate on the wire (envelope) runId — threaded through
// `reduce` — against the root run's id (state.run.runId, set by the root's
// run.started; a subagent's run.started took the spawnedByItemId branch and
// never overwrote it). A CONFIRMED-different runId ⇒ subagent: record it on the
// timeline and leave the root run untouched — idling it or running
// settleOpenInterrupts here would falsely stop the still-in-flight root and drop
// its live HITL cards.
//
// A subagent end is always terminal (completed / error / canceled), never
// interrupt: the backend surfaces a child parked for HITL as a "waiting" tool
// result to the *parent* LLM (which re-plans), not as a wire interrupt on the
// child run — see lynx agent/runtime/agent_tool.go. So there is no subagent
// interrupt card to materialize here. An unknown runId (synthetic / pre-root
// events) or a match ⇒ the root/continuation run — full handling below.
function onRunFinished(state: AgentViewState, outcome: RunOutcome, runId?: string): AgentViewState {
  if (runId && state.run.runId && runId !== state.run.runId) {
    return appendTimelineEntry({
      kind: "run-end",
      runId,
      status: outcome.type === "completed" ? "ok" : undefined,
      summary: "subagent",
    })(state);
  }
  const idle: AgentViewState = { ...state, run: { ...state.run, running: false } };
  if (outcome.type === "interrupt") {
    const parentRunId = (state.run.runId ?? "") as OpenInterrupt["parentRunId"];
    // Re-delivery (reconnect / replay) can re-present an interrupt already
    // open. Only enroll genuinely-new interrupts in a fresh envelope (else
    // openInterrupts grows duplicates); materializeInterrupt below is an
    // idempotent upsert, so re-affirming an existing card is safe.
    const openItemIds = new Set(
      idle.openInterrupts.flatMap((oi) => oi.interrupts.map((i) => i.itemId)),
    );
    const fresh = outcome.interrupts.filter((it) => !openItemIds.has(it.itemId));
    let next: AgentViewState =
      fresh.length === 0
        ? idle
        : {
            ...idle,
            openInterrupts: [
              ...idle.openInterrupts,
              {
                parentRunId,
                sessionId: (state.run.sessionId ?? "") as OpenInterrupt["sessionId"],
                interrupts: fresh,
                createdAt: new Date().toISOString(),
              },
            ],
          };
    for (const it of outcome.interrupts) next = materializeInterrupt(next, it, parentRunId);
    return next;
  }

  const { result } = outcome;
  const usage = result?.usage;
  const tokensUsed = (usage?.inputTokens ?? 0) + (usage?.outputTokens ?? 0);
  // Total cost reads usage.costUsd — there is no RunResult.costUsd (§4.2).
  const costUsd = usage?.costUsd;
  // A terminal end (completed / error / canceled / maxSteps) means any
  // interrupt still open was never resolved by the user — the run that owned
  // it is done (canceled, errored, or auto-resolved server-side). Drop those
  // open interrupts and downgrade their still-actionable cards so the UI stops
  // offering buttons that would resume a dead run.
  const withRun = settleOpenInterrupts(
    patchRun({
      running: false,
      step: result?.steps ?? state.run.step,
      totalSteps: result?.steps ?? state.run.totalSteps,
      tokens: { used: String(tokensUsed), total: state.run.tokens.total },
      cost: costUsd !== undefined ? costUsd.toFixed(2) : state.run.cost,
    })(idle),
  );

  if (outcome.type === "error") {
    const errored: AgentViewState = {
      ...withRun,
      error: {
        message: result?.error?.detail ?? result?.error?.type ?? "run failed",
        code: result?.error?.type,
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

// item.started

function onItemStarted(state: AgentViewState, item: Item): AgentViewState {
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item);
      return appendTimelineEntry({ kind: "tool-start", refId: item.id, summary: tool.fn })(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setPlan(mapPlan(item.steps))(state);
  }
}

// item.delta

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

// item.completed

function onItemCompleted(state: AgentViewState, rawItem: Item): AgentViewState {
  // item.completed ⟹ the item has settled, so its status is terminal
  // (completed | incomplete) — never running. A non-terminal status here
  // means the item was never cleanly finished: history hydration (items.list)
  // of a run lost to a crash/restart still returns its last item as
  // running (the backend reconciles this at the RunRef level, which this
  // item-based fold never reads). Coerce it to incomplete so it renders as a
  // truncated block, not a block that spins forever waiting for a live stream
  // that will never come.
  const item: Item = rawItem.status === "running" ? { ...rawItem, status: "incomplete" } : rawItem;
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
      const { state: next, tool } = writeToolCall(state, item);
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

// state.*

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

// Registration table — one StreamEventHandler per first-class event type.

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
  bind("run.started", (s, ev) => onRunStarted(s, ev.run)),
  bind("run.progress", (s, ev) => onRunProgress(s, ev.progress)),
  bind("run.finished", (s, ev, runId) => onRunFinished(s, ev.outcome, runId)),
  bind("item.started", (s, ev) => onItemStarted(s, ev.item)),
  bind("item.delta", (s, ev) => onItemDelta(s, ev.itemId, ev.delta)),
  bind("item.completed", (s, ev) => onItemCompleted(s, ev.item)),
  bind("state.snapshot", (s, ev) => onStateSnapshot(s, ev.state)),
  bind("state.delta", (s, ev) => onStateDelta(s, ev.patch as Operation[])),
];
