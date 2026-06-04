// Stateful folds — immutable (state, …) → state updates that place projected
// Items into AgentViewState. The pure wire→view mappers they build on live in
// `projections.ts`; the StreamEvent dispatch that calls these is `handlers.ts`.

import type { Item } from "@/rpc";
import type {
  AgentViewState,
  BlockStatus,
  ContentBlock,
  Message,
  ToolCall,
} from "@/protocol/run/viewState";
import {
  argsText,
  contentText,
  formatTime,
  mapQuestion,
  nameForRole,
  toolFields,
  toolLabel,
  toolStatus,
} from "./projections";

// ---------------------------------------------------------------------------
// Message / block mutations
// ---------------------------------------------------------------------------

function mutateMessage(
  state: AgentViewState,
  id: string,
  fn: (m: Message) => Message,
): AgentViewState {
  return { ...state, messages: state.messages.map((m) => (m.id === id ? fn(m) : m)) };
}

/** Ensure an open assistant-turn message exists; return its id + next state. */
function ensureTurn(state: AgentViewState, itemId: string): { state: AgentViewState; id: string } {
  const open =
    state.turnMessageId && state.messages.some((m) => m.id === state.turnMessageId)
      ? state.turnMessageId
      : null;
  if (open) return { state, id: open };
  const id = `turn:${itemId}`;
  const msg: Message = {
    id,
    role: "assistant",
    who: nameForRole("assistant"),
    time: formatTime(),
    blocks: [],
  };
  return { state: { ...state, messages: [...state.messages, msg], turnMessageId: id }, id };
}

/** Append a block to the current assistant turn (creating the turn if needed). */
export function appendToTurn(
  state: AgentViewState,
  itemId: string,
  block: ContentBlock,
): AgentViewState {
  const { state: s, id } = ensureTurn(state, itemId);
  return mutateMessage(s, id, (m) => ({ ...m, blocks: [...m.blocks, block] }));
}

/** Patch the first content block matching `match`, across all messages. */
export function patchBlock(
  state: AgentViewState,
  match: (b: ContentBlock) => boolean,
  patch: (b: ContentBlock) => ContentBlock,
): AgentViewState {
  let done = false;
  return {
    ...state,
    messages: state.messages.map((m) => {
      if (done || !m.blocks.some(match)) return m;
      done = true;
      return { ...m, blocks: m.blocks.map((b) => (match(b) ? patch(b) : b)) };
    }),
  };
}

/** Upsert: patch the matching block if present, else append a fresh one to
 *  the turn. Used by item.completed handlers (item.started may have been
 *  missed on durable replay / history hydration). */
function upsertBlock(
  state: AgentViewState,
  itemId: string,
  match: (b: ContentBlock) => boolean,
  make: () => ContentBlock,
  patch: (b: ContentBlock) => ContentBlock,
): AgentViewState {
  if (state.messages.some((m) => m.blocks.some(match))) return patchBlock(state, match, patch);
  return appendToTurn(state, itemId, make());
}

export function updateTool(
  state: AgentViewState,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentViewState {
  const existing = state.toolCalls[id];
  if (!existing) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

// ---------------------------------------------------------------------------
// Per-item folds — shared by item.started (append) and item.completed
// (upsert). started/completed differ only in the block status they stamp,
// so both call through here; the upsert keeps durable replay / history
// hydration idempotent (a re-seen item patches in place, never duplicates).
// ---------------------------------------------------------------------------

type ItemOf<T extends Item["type"]> = Extract<Item, { type: T }>;

/** Append a user-message bubble (opens a fresh assistant turn). Idempotent —
 *  a re-seen id is a no-op, dodging React's duplicate-key warning — and it
 *  reconciles the optimistic placeholder so the streamed item doesn't double. */
export function appendUserMessage(
  state: AgentViewState,
  item: ItemOf<"userMessage">,
  status: BlockStatus,
): AgentViewState {
  // Already have this exact item (started→completed re-seen, or durable
  // replay / history hydration) → no-op.
  if (state.messages.some((m) => m.id === item.id)) return state;
  const text = contentText(item.content);
  // Reconcile the optimistic placeholder: send() renders the user's bubble
  // immediately with a local-* id, a round-trip before the runtime streams
  // the real userMessage Item (with its own server id). Upgrade the oldest
  // matching placeholder's id in place rather than appending a duplicate.
  const placeholder = state.messages.findIndex(
    (m) =>
      m.role === "user" &&
      m.id.startsWith("local-") &&
      m.blocks.find((b): b is Extract<ContentBlock, { kind: "text" }> => b.kind === "text")
        ?.text === text,
  );
  if (placeholder !== -1) {
    const messages = state.messages.map((m, i) => (i === placeholder ? { ...m, id: item.id } : m));
    return { ...state, messages, turnMessageId: null };
  }
  const msg: Message = {
    id: item.id,
    role: "user",
    who: nameForRole("user"),
    time: formatTime(item.createdAt),
    blocks: [{ kind: "text", text, status }],
  };
  return { ...state, messages: [...state.messages, msg], turnMessageId: null };
}

/** Upsert the agentMessage text block for an item. */
export function foldText(
  state: AgentViewState,
  item: ItemOf<"agentMessage">,
  status: BlockStatus,
): AgentViewState {
  const text = contentText(item.content);
  return upsertBlock(
    state,
    item.id,
    (b) => b.kind === "text" && b.itemId === item.id,
    () => ({ kind: "text", itemId: item.id, text, status }),
    (b) => (b.kind === "text" ? { ...b, text, status } : b),
  );
}

/** Upsert the reasoning block for an item. */
export function foldReasoning(
  state: AgentViewState,
  item: ItemOf<"reasoning">,
  status: BlockStatus,
): AgentViewState {
  // `text` is absent on the item.started shell — it streams via item.delta
  // (same as agentMessage content). Seed "" so deltas accumulate cleanly
  // instead of onto `undefined`.
  const text = item.text ?? "";
  return upsertBlock(
    state,
    item.id,
    (b) => b.kind === "reasoning" && b.reasoningId === item.id,
    () => ({ kind: "reasoning", reasoningId: item.id, text, status }),
    (b) => (b.kind === "reasoning" ? { ...b, text, status } : b),
  );
}

/** Upsert the question block for an item (only `status` changes once shown). */
export function foldQuestion(
  state: AgentViewState,
  item: ItemOf<"question">,
  status: BlockStatus,
): AgentViewState {
  return upsertBlock(
    state,
    item.id,
    (b) => b.kind === "question" && b.itemId === item.id,
    () => ({ kind: "question", status, itemId: item.id, questions: mapQuestion(item.question) }),
    (b) => (b.kind === "question" ? { ...b, status } : b),
  );
}

/** Ensure the tool block + write its toolCalls entry; preserves any
 *  accumulated arg text. Returns the next state + the resolved ToolCall (the
 *  caller stamps the matching tool-start / tool-end timeline entry). */
export function writeToolCall(
  state: AgentViewState,
  item: ItemOf<"toolCall">,
  duration: string,
): { state: AgentViewState; tool: ToolCall } {
  const withBlock =
    state.toolCalls[item.id] === undefined
      ? appendToTurn(state, item.id, { kind: "tool", toolCallId: item.id })
      : state;
  const prev = withBlock.toolCalls[item.id];
  const tool: ToolCall = {
    id: item.id,
    kind: item.tool?.kind ?? "tool",
    fn: toolLabel(item.tool),
    // Accumulated stream text wins; fall back to the parsed arguments object
    // when a generic tool delivered no toolArguments deltas (object-on-complete).
    args: (prev?.args ?? "") || argsText(item.tool),
    status: toolStatus(item),
    duration,
    // Keep the accumulated stream output (commandExecution stdout streams via
    // toolOutput) — toolFields only overrides `result` for kinds that carry a
    // structured one (generic `tool`).
    result: prev?.result,
    // Surface the tool-level failure reason (§8.1 channel b) so an "err" tool
    // tells the user *why*, not just that it went red.
    error: item.error ? (item.error.detail ?? item.error.type) : undefined,
    ...toolFields(item.tool),
  };
  return { state: { ...withBlock, toolCalls: { ...withBlock.toolCalls, [item.id]: tool } }, tool };
}
