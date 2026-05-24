// Built-in plugin: AG-UI protocol semantics.
//
// Every RUN_*, TEXT_MESSAGE_*, TOOL_CALL_*, REASONING_* case used to live
// inside `protocol/agui/reducer.ts`. Pulling them into a plugin means even
// the protocol layer is a (replaceable) extension — a power user can swap
// this for a custom dialect by registering a different `core-reducer` plugin
// that takes priority. The kernel reducer is now pure dispatch.

import { applyPatch, deepClone, type Operation } from "fast-json-patch";
import {
  EventType,
  type BaseEvent,
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
import { definePlugin, type CoreEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock, Message, ToolCall } from "@/protocol/agui/viewState";

// Erases each handler's specific event variant down to BaseEvent so a
// uniform `[EventType, CoreEventHandler]` table can carry them all. The
// per-handler signature still type-checks the event payload it
// destructures; the cast is only for the table's homogeneous shape.
function bind<E extends BaseEvent>(
  fn: (state: AgentViewState, ev: E) => AgentViewState,
): CoreEventHandler {
  return fn as CoreEventHandler;
}

// ---------------------------------------------------------------------------
// Helpers (copied from the old reducer — they're pure state ops, no I/O).
// ---------------------------------------------------------------------------

function nowTime(): string {
  const d = new Date();
  const h = d.getHours() % 12 || 12;
  const m = String(d.getMinutes()).padStart(2, "0");
  const meridiem = d.getHours() >= 12 ? "PM" : "AM";
  return `${h}:${m} ${meridiem}`;
}

function nameForRole(role: Message["role"]): string {
  if (role === "user") return "You";
  if (role === "assistant") return "Sonnet 4.5";
  return "System";
}

function updateMessage(
  state: AgentViewState,
  id: string,
  fn: (m: Message) => Message,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => (m.id === id ? fn(m) : m)),
  };
}

function updateTool(
  state: AgentViewState,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentViewState {
  const existing = state.toolCalls[id];
  if (!existing) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

function appendBlock(m: Message, block: ContentBlock): Message {
  return { ...m, blocks: [...m.blocks, block] };
}

function appendTextDelta(m: Message, delta: string): Message {
  const blocks = m.blocks.slice();
  const last = blocks[blocks.length - 1];
  if (last && last.kind === "text" && last.streaming) {
    blocks[blocks.length - 1] = { ...last, text: last.text + delta };
    return { ...m, blocks };
  }
  blocks.push({ kind: "text", text: delta, streaming: true });
  return { ...m, blocks };
}

function mapReasoning(
  state: AgentViewState,
  reasoningId: string,
  fn: (b: Extract<ContentBlock, { kind: "reasoning" }>) => ContentBlock,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => {
      let touched = false;
      const blocks = m.blocks.map((b) => {
        if (b.kind !== "reasoning" || b.reasoningId !== reasoningId) return b;
        touched = true;
        return fn(b);
      });
      return touched ? { ...m, blocks } : m;
    }),
  };
}

function findLastAssistantMessageId(state: AgentViewState): string | null {
  for (let i = state.messages.length - 1; i >= 0; i--) {
    if (state.messages[i].role === "assistant") return state.messages[i].id;
  }
  return null;
}

// ---------------------------------------------------------------------------
// Per-event handlers — pure (state, event) → state.
// ---------------------------------------------------------------------------

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

// STEP_FINISHED: the named step the agent announced via STEP_STARTED is
// done. We mirror onStepStarted's effect on `activity` (the topbar
// "what is the agent doing" pill) by clearing it — keeping the old step
// name visible after it finishes is misleading. Step counter bumps so
// downstream UI can show "step N / M" if it cares.
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

// ---------------------------------------------------------------------------
// CHUNK variants — combined START/CONTENT/END streams.
// ---------------------------------------------------------------------------
//
// AG-UI's *_CHUNK events let a backend emit one event per delta with optional
// "first chunk" metadata (role, messageId, toolCallName) and a payload delta.
// There's no explicit END for chunked streams — closure rides on RUN_FINISHED
// or a follow-up non-chunk END event.
//
// Strategy: do the START detection inline. If the entity (message / tool
// call / reasoning block) isn't present yet, materialize it; then merge
// the delta. No synthetic events crossing the dispatcher — this is the
// same handler approach as the non-chunk variants, just with the START
// fused into the same call as CONTENT.

function findMessageById(state: AgentViewState, id: string): Message | undefined {
  return state.messages.find((m) => m.id === id);
}

const onTextChunk = (state: AgentViewState, ev: TextMessageChunkEvent): AgentViewState => {
  if (!ev.messageId) return state;
  let next = state;
  if (!findMessageById(next, ev.messageId)) {
    // First chunk for this messageId — materialize the message.
    const role: Message["role"] =
      ev.role === "user" ? "user" : ev.role === "system" ? "system" : "assistant";
    next = {
      ...next,
      messages: [
        ...next.messages,
        {
          id: ev.messageId,
          role,
          who: nameForRole(role),
          time: nowTime(),
          blocks: [],
        },
      ],
    };
  }
  if (ev.delta) {
    next = updateMessage(next, ev.messageId, (m) => appendTextDelta(m, ev.delta!));
  }
  return next;
};

const onToolChunk = (state: AgentViewState, ev: ToolCallChunkEvent): AgentViewState => {
  if (!ev.toolCallId) return state;
  let next = state;
  if (!next.toolCalls[ev.toolCallId]) {
    // First chunk — synthesize the tool entry. toolCallName might be
    // absent (some backends only set it on the first chunk that has it);
    // we fall back to "" and downstream consumers tolerate that until
    // a later chunk fills it.
    next = {
      ...next,
      toolCalls: {
        ...next.toolCalls,
        [ev.toolCallId]: {
          id: ev.toolCallId,
          fn: ev.toolCallName ?? "",
          args: "",
          status: "running",
          duration: "LIVE",
        },
      },
    };
    if (ev.parentMessageId) {
      next = updateMessage(next, ev.parentMessageId, (m) =>
        appendBlock(m, { kind: "tool", toolCallId: ev.toolCallId! }),
      );
    }
  } else if (ev.toolCallName && !next.toolCalls[ev.toolCallId].fn) {
    // Later chunk arrived with the name — fill in the gap.
    next = {
      ...next,
      toolCalls: {
        ...next.toolCalls,
        [ev.toolCallId]: { ...next.toolCalls[ev.toolCallId], fn: ev.toolCallName },
      },
    };
  }
  if (ev.delta) {
    next = updateTool(next, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));
  }
  return next;
};

// ---------------------------------------------------------------------------
// THINKING_* — extended-thinking phase events (Claude 3.7+ style).
// ---------------------------------------------------------------------------
//
// Two layers:
//   THINKING_START / THINKING_END           — phase markers (optional title)
//   THINKING_TEXT_MESSAGE_START/CONTENT/END — the actual text inside
//
// THINKING_TEXT_MESSAGE_* events don't carry messageId — they're an
// implicitly-scoped sequence between THINKING_START and THINKING_END. We
// translate them into our reasoning-block model: each START opens a new
// reasoning block on the last assistant message with a synthetic id, and
// CONTENT/END operate on the most recent still-streaming reasoning block.
// Visually identical to REASONING_MESSAGE_* — same collapsible "thought"
// panel.
//
// The THINKING_START/END phase markers themselves do nothing: the inner
// blocks already convey "thinking happened" via their stream lifecycle.
// If we later want to expose the optional `title` from THINKING_START,
// it'd hang on the reasoning block as a separate field.

let thinkingIdCounter = 0;
function nextThinkingId(): string {
  thinkingIdCounter += 1;
  return `thinking:${Date.now()}:${thinkingIdCounter}`;
}

function findActiveThinkingId(state: AgentViewState): string | null {
  // Walk messages backwards for the most recent still-streaming reasoning
  // block. That's the "currently open" thinking block we should write to.
  for (let i = state.messages.length - 1; i >= 0; i--) {
    const m = state.messages[i];
    for (let j = m.blocks.length - 1; j >= 0; j--) {
      const b = m.blocks[j];
      if (b.kind === "reasoning" && b.streaming) return b.reasoningId;
    }
  }
  return null;
}

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

// ---------------------------------------------------------------------------
// STATE_* — backend-owned shared state.
// ---------------------------------------------------------------------------
//
// STATE_SNAPSHOT replaces `state.shared` wholesale. STATE_DELTA applies
// a JSON Patch (RFC 6902) array to it. Plugins subscribe via
// useSharedState() in the SDK selectors layer. A throwing patch (path
// not found, op invalid) is logged and the state is left unchanged —
// silently swallowing a bad patch is better than throwing all the way
// up the reducer chain and crashing the chat.
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
    // eslint-disable-next-line no-console
    console.error("[agui] STATE_DELTA patch failed:", err);
    return state;
  }
};

// ---------------------------------------------------------------------------
// ACTIVITY_* — structured per-message activity streams.
// ---------------------------------------------------------------------------
//
// Each event is scoped by (messageId, activityType). SNAPSHOT writes the
// content blob; the optional `replace` flag controls whether existing
// content for the same key is replaced (default false → shallow merge).
// DELTA applies a JSON Patch to that content. Renderers in plugins pick
// the activity types they understand and ignore the rest.
function updateActivity(
  state: AgentViewState,
  messageId: string,
  activityType: string,
  fn: (prev: unknown) => unknown,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => {
      if (m.id !== messageId) return m;
      const prev = m.activities?.[activityType];
      const next = fn(prev);
      return {
        ...m,
        activities: { ...(m.activities ?? {}), [activityType]: next },
      };
    }),
  };
}

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
      // eslint-disable-next-line no-console
      console.error(
        `[agui] ACTIVITY_DELTA patch failed for ${ev.messageId}/${ev.activityType}:`,
        err,
      );
      return prev;
    }
  });
};

const onReasoningChunk = (
  state: AgentViewState,
  ev: ReasoningMessageChunkEvent,
): AgentViewState => {
  if (!ev.messageId) return state;
  // Has this reasoning block been opened yet on any message?
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

// MESSAGES_SNAPSHOT — full conversation hydrate. Sent on reconnect or
// when joining an existing thread. Replaces both `messages` and
// `toolCalls` wholesale (the snapshot is authoritative); leaves run /
// plan / error untouched so the UI doesn't lose stop-button + plan
// context if it was already mid-run.
//
// Conversion notes:
//   - developer / system messages collapse into our "system" role.
//   - assistant.toolCalls (the OpenAI-shaped {id, function: {name, arguments}})
//     each become both a tool block on the assistant message AND an
//     entry in state.toolCalls. We default to status "ok" + duration "—"
//     because the snapshot represents settled history; a still-running
//     tool would arrive via fresh TOOL_CALL_START events after the snap.
//   - role:"tool" messages don't become UI messages — they're tool
//     RESULTS, so we fold them into toolCalls[toolCallId].result and
//     adjust status if `error` is set.
type SnapshotMessage = MessagesSnapshotEvent["messages"][number];
type SnapToolCall = {
  id: string;
  type: "function";
  function: { name: string; arguments: string };
};

const onMessagesSnapshot = (state: AgentViewState, ev: MessagesSnapshotEvent): AgentViewState => {
  const messages: Message[] = [];
  const toolCalls: Record<string, ToolCall> = {};

  for (const m of ev.messages as SnapshotMessage[]) {
    if (m.role === "tool") {
      // Tool result — attach to its tool call entry. If the matching
      // tool call hasn't been seen yet (out-of-order snapshot), stash
      // a minimal entry; downstream assistant message will fill in fn.
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
      continue;
    }

    const role: Message["role"] =
      m.role === "user" ? "user" : m.role === "assistant" ? "assistant" : "system";

    const blocks: ContentBlock[] = [];
    if (m.role === "assistant") {
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
    } else {
      const content = (m as { content: string }).content;
      blocks.push({ kind: "text", text: content, streaming: false });
    }

    messages.push({
      id: m.id,
      role,
      who: nameForRole(role),
      // Snapshot messages don't carry timestamps in the AG-UI message
      // type — we use "now" as a stand-in. Real backends should include
      // timestamps when they extend the message schema.
      time: nowTime(),
      blocks,
    });
  }

  return { ...state, messages, toolCalls };
};

// ---------------------------------------------------------------------------
// Dispatch table
// ---------------------------------------------------------------------------
//
// Every AG-UI core event the plugin handles, paired with its handler.
// Adding a new event = one row here + a new `onX` function above. The
// `setup` block iterates the table — no per-event registration code to
// keep in sync. THINKING_START / THINKING_END / REASONING_START /
// REASONING_END phase markers are deliberately absent: the inner
// MESSAGE stream lifecycle already conveys those.

const HANDLERS: ReadonlyArray<[EventType, CoreEventHandler]> = [
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

export default definePlugin({
  name: "lyra.builtin.core-reducer",
  version: "1.0.0",
  setup({ host }) {
    for (const [type, handler] of HANDLERS) {
      host.agui.onCore(type, handler);
    }
  },
});
