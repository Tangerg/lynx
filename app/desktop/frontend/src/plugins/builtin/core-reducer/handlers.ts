// AG-UI per-event handlers + dispatch table. Each handler is a pure
// (state, event) → state mapping; the table at the bottom is what
// the plugin registers with `host.agui.onCore`.

import { applyPatch, deepClone, type Operation } from "fast-json-patch";
import {
  EventType,
  type ActivityDeltaEvent,
  type ActivitySnapshotEvent,
  type MessagesSnapshotEvent,
  type ReasoningMessageChunkEvent,
  type ReasoningMessageContentEvent,
  type ReasoningMessageEndEvent,
  type ReasoningMessageStartEvent,
  type RunErrorEvent,
  type RunStartedEvent,
  type StateDeltaEvent,
  type StateSnapshotEvent,
  type StepFinishedEvent,
  type StepStartedEvent,
  type TextMessageChunkEvent,
  type TextMessageContentEvent,
  type TextMessageEndEvent,
  type TextMessageStartEvent,
  type ThinkingTextMessageContentEvent,
  type ToolCallArgsEvent,
  type ToolCallChunkEvent,
  type ToolCallEndEvent,
  type ToolCallResultEvent,
  type ToolCallStartEvent,
} from "@ag-ui/core";
import type { CoreEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock, Message, ToolCall } from "@/protocol/agui/viewState";
import {
  appendBlock,
  appendTextDelta,
  bind,
  findActiveThinkingId,
  findLastAssistantMessageId,
  findMessageById,
  mapReasoning,
  nameForRole,
  nextThinkingId,
  nowTime,
  updateActivity,
  updateMessage,
  updateTool,
} from "./helpers";

// ---- run lifecycle ------------------------------------------------------

const onRunStarted = (state: AgentViewState, ev: RunStartedEvent): AgentViewState => ({
  ...state,
  // Clear the previous run's error banner on a fresh start — once the
  // agent is moving again, the stale message would just confuse.
  error: null,
  run: { ...state.run, running: true, threadId: ev.threadId, runId: ev.runId },
});

const onRunError = (state: AgentViewState, ev: RunErrorEvent): AgentViewState => ({
  ...state,
  error: { message: ev.message, code: ev.code },
  run: { ...state.run, running: false, activity: "" },
});

// STEP_FINISHED clears `activity` (the topbar "what is the agent doing"
// pill) and bumps the step counter — keeping the old step name visible
// after it finishes is misleading.
const onStepFinished = (state: AgentViewState, ev: StepFinishedEvent): AgentViewState => ({
  ...state,
  run: {
    ...state.run,
    step: state.run.step + 1,
    // Only clear if it matches — defensive against out-of-order events.
    activity: state.run.activity === ev.stepName ? "" : state.run.activity,
  },
});

const onRunFinished = (state: AgentViewState): AgentViewState => ({
  ...state,
  run: { ...state.run, running: false },
});

const onStepStarted = (state: AgentViewState, ev: StepStartedEvent): AgentViewState => ({
  ...state,
  run: { ...state.run, activity: ev.stepName },
});

// ---- text messages ------------------------------------------------------

const onTextStart = (state: AgentViewState, ev: TextMessageStartEvent): AgentViewState => {
  const role: Message["role"] =
    ev.role === "user" ? "user" : ev.role === "system" ? "system" : "assistant";
  const msg: Message = {
    id: ev.messageId,
    role,
    who: nameForRole(role),
    time: nowTime(),
    blocks: [],
  };
  return { ...state, messages: [...state.messages, msg] };
};

const onTextContent = (state: AgentViewState, ev: TextMessageContentEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => appendTextDelta(m, ev.delta));

const onTextEnd = (state: AgentViewState, ev: TextMessageEndEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => ({
    ...m,
    blocks: m.blocks.map((b) =>
      b.kind === "text" && b.streaming ? { ...b, streaming: false } : b,
    ),
  }));

// First chunk for this messageId materializes the message; later chunks
// just append. No explicit END — closure rides on RUN_FINISHED or a
// later non-chunk END event.
const onTextChunk = (state: AgentViewState, ev: TextMessageChunkEvent): AgentViewState => {
  if (!ev.messageId) return state;
  let next = state;
  if (!findMessageById(next, ev.messageId)) {
    const role: Message["role"] =
      ev.role === "user" ? "user" : ev.role === "system" ? "system" : "assistant";
    next = {
      ...next,
      messages: [
        ...next.messages,
        { id: ev.messageId, role, who: nameForRole(role), time: nowTime(), blocks: [] },
      ],
    };
  }
  if (ev.delta) {
    next = updateMessage(next, ev.messageId, (m) => appendTextDelta(m, ev.delta!));
  }
  return next;
};

// ---- tool calls ---------------------------------------------------------

const onToolStart = (state: AgentViewState, ev: ToolCallStartEvent): AgentViewState => {
  const tool: ToolCall = {
    id: ev.toolCallId,
    fn: ev.toolCallName,
    args: "",
    status: "running",
    duration: "LIVE",
  };
  let next: AgentViewState = {
    ...state,
    toolCalls: { ...state.toolCalls, [ev.toolCallId]: tool },
  };
  if (ev.parentMessageId) {
    next = updateMessage(next, ev.parentMessageId, (m) =>
      appendBlock(m, { kind: "tool", toolCallId: ev.toolCallId }),
    );
  }
  return next;
};

const onToolArgs = (state: AgentViewState, ev: ToolCallArgsEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));

const onToolEnd = (state: AgentViewState, ev: ToolCallEndEvent): AgentViewState => {
  // AG-UI's TOOL_CALL_END is minimal — most "result" data rides on
  // TOOL_CALL_RESULT or comes via summary fields we surface as extras.
  const ex = ev as ToolCallEndEvent & {
    status?: ToolCall["status"];
    durationMs?: number;
    added?: number;
    removed?: number;
    hits?: number;
    lines?: number;
  };
  return updateTool(state, ev.toolCallId, (t) => ({
    ...t,
    status: ex.status ?? "ok",
    duration:
      ex.durationMs != null ? `${ex.durationMs}ms` : t.duration === "LIVE" ? "—" : t.duration,
    added: ex.added ?? t.added,
    removed: ex.removed ?? t.removed,
    hits: ex.hits ?? t.hits,
    lines: ex.lines ?? t.lines,
  }));
};

const onToolResult = (state: AgentViewState, ev: ToolCallResultEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, result: ev.content }));

// First chunk — synthesize the tool entry. toolCallName might be absent
// (some backends only set it on the first chunk that carries it); we fall
// back to "" and downstream consumers tolerate that until a later chunk
// fills it.
function initToolCall(state: AgentViewState, ev: ToolCallChunkEvent): AgentViewState {
  const id = ev.toolCallId!;
  let next: AgentViewState = {
    ...state,
    toolCalls: {
      ...state.toolCalls,
      [id]: {
        id,
        fn: ev.toolCallName ?? "",
        args: "",
        status: "running",
        duration: "LIVE",
      },
    },
  };
  if (ev.parentMessageId) {
    next = updateMessage(next, ev.parentMessageId, (m) =>
      appendBlock(m, { kind: "tool", toolCallId: id }),
    );
  }
  return next;
}

function fillToolName(state: AgentViewState, id: string, name: string): AgentViewState {
  return {
    ...state,
    toolCalls: {
      ...state.toolCalls,
      [id]: { ...state.toolCalls[id], fn: name },
    },
  };
}

const onToolChunk = (state: AgentViewState, ev: ToolCallChunkEvent): AgentViewState => {
  if (!ev.toolCallId) return state;
  let next = state;
  const existing = next.toolCalls[ev.toolCallId];
  if (!existing) {
    next = initToolCall(next, ev);
  } else if (ev.toolCallName && !existing.fn) {
    next = fillToolName(next, ev.toolCallId, ev.toolCallName);
  }
  if (ev.delta) {
    next = updateTool(next, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));
  }
  return next;
};

// ---- reasoning ----------------------------------------------------------

const onReasoningStart = (
  state: AgentViewState,
  ev: ReasoningMessageStartEvent,
): AgentViewState => {
  const parentId = (ev as ReasoningMessageStartEvent & { parentMessageId?: string })
    .parentMessageId;
  const targetId = parentId ?? findLastAssistantMessageId(state);
  if (!targetId) return state;
  return updateMessage(state, targetId, (m) =>
    appendBlock(m, { kind: "reasoning", reasoningId: ev.messageId, text: "", streaming: true }),
  );
};

const onReasoningContent = (
  state: AgentViewState,
  ev: ReasoningMessageContentEvent,
): AgentViewState => mapReasoning(state, ev.messageId, (b) => ({ ...b, text: b.text + ev.delta }));

const onReasoningEnd = (state: AgentViewState, ev: ReasoningMessageEndEvent): AgentViewState =>
  mapReasoning(state, ev.messageId, (b) => ({ ...b, streaming: false }));

const onReasoningChunk = (
  state: AgentViewState,
  ev: ReasoningMessageChunkEvent,
): AgentViewState => {
  if (!ev.messageId) return state;
  const exists = state.messages.some((m) =>
    m.blocks.some((b) => b.kind === "reasoning" && b.reasoningId === ev.messageId),
  );
  let next = state;
  if (!exists) {
    const parentId =
      (ev as ReasoningMessageChunkEvent & { parentMessageId?: string }).parentMessageId ??
      findLastAssistantMessageId(next);
    if (!parentId) return state;
    next = updateMessage(next, parentId, (m) =>
      appendBlock(m, {
        kind: "reasoning",
        reasoningId: ev.messageId!,
        text: "",
        streaming: true,
      }),
    );
  }
  if (ev.delta) {
    next = mapReasoning(next, ev.messageId, (b) => ({ ...b, text: b.text + ev.delta }));
  }
  return next;
};

// ---- extended-thinking (Claude 3.7+) ------------------------------------
//
// THINKING_TEXT_MESSAGE_* events have no messageId; they're scoped by the
// surrounding THINKING_START/END pair. We translate each START into a
// reasoning block (synthetic id) on the last assistant message;
// CONTENT/END operate on the most recent open block. THINKING_START/END
// themselves are no-ops — the inner stream lifecycle conveys "thinking
// happened".

const onThinkingTextStart = (state: AgentViewState): AgentViewState => {
  const parentId = findLastAssistantMessageId(state);
  if (!parentId) return state;
  return updateMessage(state, parentId, (m) =>
    appendBlock(m, {
      kind: "reasoning",
      reasoningId: nextThinkingId(),
      text: "",
      streaming: true,
    }),
  );
};

const onThinkingTextContent = (
  state: AgentViewState,
  ev: ThinkingTextMessageContentEvent,
): AgentViewState => {
  const id = findActiveThinkingId(state);
  if (!id) return state;
  return mapReasoning(state, id, (b) => ({ ...b, text: b.text + ev.delta }));
};

const onThinkingTextEnd = (state: AgentViewState): AgentViewState => {
  const id = findActiveThinkingId(state);
  if (!id) return state;
  return mapReasoning(state, id, (b) => ({ ...b, streaming: false }));
};

// ---- shared state (STATE_*) ---------------------------------------------
//
// STATE_SNAPSHOT replaces `state.shared` wholesale. STATE_DELTA applies
// a JSON Patch (RFC 6902) array to it. A throwing patch is logged and
// the state left unchanged — silently swallowing a bad patch beats
// crashing the chat.

const onStateSnapshot = (state: AgentViewState, ev: StateSnapshotEvent): AgentViewState => {
  const snapshot = (ev as { snapshot?: unknown }).snapshot;
  if (snapshot == null || typeof snapshot !== "object") return state;
  return { ...state, shared: snapshot as Record<string, unknown> };
};

const onStateDelta = (state: AgentViewState, ev: StateDeltaEvent): AgentViewState => {
  const patch = (ev as { delta?: unknown[] }).delta;
  if (!Array.isArray(patch) || patch.length === 0) return state;
  try {
    // Clone to keep our `shared` immutable across the reduction step
    // — fast-json-patch mutates the document in place otherwise.
    const next = applyPatch(
      deepClone(state.shared),
      patch as Operation[],
      /* validate */ false,
      /* mutateDocument */ true,
    ).newDocument as Record<string, unknown>;
    return { ...state, shared: next };
  } catch (err) {
    console.error("[agui] STATE_DELTA patch failed:", err);
    return state;
  }
};

// ---- per-message activity streams (ACTIVITY_*) --------------------------
//
// Each event is scoped by (messageId, activityType). SNAPSHOT writes the
// content blob; the optional `replace` flag controls whether existing
// content for the same key is replaced (default false → shallow merge).
// DELTA applies a JSON Patch. Renderers pick the activity types they
// understand and ignore the rest.

const onActivitySnapshot = (state: AgentViewState, ev: ActivitySnapshotEvent): AgentViewState => {
  const content = (ev as { content?: Record<string, unknown> }).content ?? {};
  const replace = Boolean((ev as { replace?: boolean }).replace);
  return updateActivity(state, ev.messageId, ev.activityType, (prev) => {
    if (replace || !prev || typeof prev !== "object") return content;
    return { ...(prev as Record<string, unknown>), ...content };
  });
};

const onActivityDelta = (state: AgentViewState, ev: ActivityDeltaEvent): AgentViewState => {
  const patch = (ev as { patch?: unknown[] }).patch;
  if (!Array.isArray(patch) || patch.length === 0) return state;
  return updateActivity(state, ev.messageId, ev.activityType, (prev) => {
    try {
      const base = prev && typeof prev === "object" ? deepClone(prev) : {};
      return applyPatch(base, patch as Operation[], false, true).newDocument;
    } catch (err) {
      console.error(
        `[agui] ACTIVITY_DELTA patch failed for ${ev.messageId}/${ev.activityType}:`,
        err,
      );
      return prev;
    }
  });
};

// ---- MESSAGES_SNAPSHOT (bulk hydrate on reconnect / thread switch) ------
//
// Replaces messages + toolCalls wholesale (snapshot is authoritative);
// leaves run / plan / error so a mid-run UI keeps its stop button + plan.
// developer / system both collapse to "system"; role:"tool" messages fold
// into toolCalls[id].result (they're results, not chat turns). Snapshot
// tools default to "ok" + duration "—" since they represent settled
// history.

type SnapshotMessage = MessagesSnapshotEvent["messages"][number];
type SnapToolCall = {
  id: string;
  type: "function";
  function: { name: string; arguments: string };
};

// Tool result — attach to its tool call entry. If the matching tool call
// hasn't been seen yet (out-of-order snapshot), stash a minimal entry;
// the downstream assistant message will fill in fn.
function ingestToolResult(toolCalls: Record<string, ToolCall>, m: SnapshotMessage): void {
  const tcId = (m as { toolCallId: string }).toolCallId;
  const content = (m as { content: string }).content;
  const errored = Boolean((m as { error?: string }).error);
  const prev = toolCalls[tcId];
  toolCalls[tcId] = {
    id: tcId,
    fn: prev?.fn ?? "",
    args: prev?.args ?? "",
    status: errored ? "err" : "ok",
    duration: prev?.duration ?? "—",
    result: content,
  };
}

// Build assistant blocks (text + tool placeholders) and side-effect the
// accumulator with each tool call's metadata.
function buildAssistantBlocks(
  m: SnapshotMessage,
  toolCalls: Record<string, ToolCall>,
): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  const content = (m as { content?: string }).content;
  if (content) {
    blocks.push({ kind: "text", text: content, streaming: false });
  }
  const tcs = (m as { toolCalls?: SnapToolCall[] }).toolCalls ?? [];
  for (const tc of tcs) {
    blocks.push({ kind: "tool", toolCallId: tc.id });
    const prev = toolCalls[tc.id];
    toolCalls[tc.id] = {
      id: tc.id,
      fn: tc.function.name,
      args: tc.function.arguments,
      status: prev?.status ?? "ok",
      duration: prev?.duration ?? "—",
      result: prev?.result,
    };
  }
  return blocks;
}

const onMessagesSnapshot = (state: AgentViewState, ev: MessagesSnapshotEvent): AgentViewState => {
  const messages: Message[] = [];
  const toolCalls: Record<string, ToolCall> = {};

  for (const m of ev.messages as SnapshotMessage[]) {
    if (m.role === "tool") {
      ingestToolResult(toolCalls, m);
      continue;
    }

    const role: Message["role"] =
      m.role === "user" ? "user" : m.role === "assistant" ? "assistant" : "system";

    const blocks: ContentBlock[] =
      m.role === "assistant"
        ? buildAssistantBlocks(m, toolCalls)
        : [{ kind: "text", text: (m as { content: string }).content, streaming: false }];

    messages.push({
      id: m.id,
      role,
      who: nameForRole(role),
      // Snapshot messages don't carry timestamps in the AG-UI schema — we
      // use "now" as a stand-in. Real backends should include timestamps.
      time: nowTime(),
      blocks,
    });
  }

  return { ...state, messages, toolCalls };
};

// ---- dispatch table -----------------------------------------------------
//
// Every AG-UI core event the plugin handles, paired with its handler.
// Adding a new event = one row here + a new `onX` function above. The
// plugin's setup iterates the table — no per-event registration code to
// keep in sync. THINKING_START / THINKING_END / REASONING_START /
// REASONING_END phase markers are deliberately absent: the inner MESSAGE
// stream lifecycle already conveys those.

export const HANDLERS: ReadonlyArray<[EventType, CoreEventHandler]> = [
  // Run lifecycle.
  [EventType.RUN_STARTED, bind(onRunStarted)],
  [EventType.RUN_FINISHED, bind(onRunFinished)],
  [EventType.RUN_ERROR, bind(onRunError)],
  [EventType.STEP_STARTED, bind(onStepStarted)],
  [EventType.STEP_FINISHED, bind(onStepFinished)],

  // Text messages — including the fused CHUNK variant.
  [EventType.TEXT_MESSAGE_START, bind(onTextStart)],
  [EventType.TEXT_MESSAGE_CONTENT, bind(onTextContent)],
  [EventType.TEXT_MESSAGE_END, bind(onTextEnd)],
  [EventType.TEXT_MESSAGE_CHUNK, bind(onTextChunk)],

  // Tool calls — including the fused CHUNK variant.
  [EventType.TOOL_CALL_START, bind(onToolStart)],
  [EventType.TOOL_CALL_ARGS, bind(onToolArgs)],
  [EventType.TOOL_CALL_END, bind(onToolEnd)],
  [EventType.TOOL_CALL_RESULT, bind(onToolResult)],
  [EventType.TOOL_CALL_CHUNK, bind(onToolChunk)],

  // Reasoning — including the fused CHUNK variant.
  [EventType.REASONING_MESSAGE_START, bind(onReasoningStart)],
  [EventType.REASONING_MESSAGE_CONTENT, bind(onReasoningContent)],
  [EventType.REASONING_MESSAGE_END, bind(onReasoningEnd)],
  [EventType.REASONING_MESSAGE_CHUNK, bind(onReasoningChunk)],

  // Extended-thinking phase (Claude 3.7+). Text events map onto our
  // existing reasoning-block UI.
  [EventType.THINKING_TEXT_MESSAGE_START, bind(onThinkingTextStart)],
  [EventType.THINKING_TEXT_MESSAGE_CONTENT, bind(onThinkingTextContent)],
  [EventType.THINKING_TEXT_MESSAGE_END, bind(onThinkingTextEnd)],

  // Snapshots — bulk hydration on reconnect / thread switch.
  [EventType.MESSAGES_SNAPSHOT, bind(onMessagesSnapshot)],

  // Shared state — STATE_SNAPSHOT replaces wholesale; STATE_DELTA applies
  // JSON Patch. Plugins consume via useSharedState().
  [EventType.STATE_SNAPSHOT, bind(onStateSnapshot)],
  [EventType.STATE_DELTA, bind(onStateDelta)],

  // Per-message activity streams — structured side-data scoped by
  // (messageId, activityType). Renderers pick the types they know.
  [EventType.ACTIVITY_SNAPSHOT, bind(onActivitySnapshot)],
  [EventType.ACTIVITY_DELTA, bind(onActivityDelta)],
];
