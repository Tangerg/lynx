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
import { isLocalMessageId } from "@/protocol/run/viewState";
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

/** Drop every open interrupt and downgrade its still-actionable approval /
 *  question card to `incomplete`. Called on a terminal run end (not an
 *  interrupt): the run that owned the interrupt is finished, so a card left in
 *  `requires-action` would offer buttons that resume a dead run. No-op when
 *  nothing is open (a resolved interrupt already emptied the list). */
export function settleOpenInterrupts(state: AgentViewState): AgentViewState {
  if (state.openInterrupts.length === 0) return state;
  const actionable = (b: ContentBlock) =>
    (b.kind === "approval" || b.kind === "question") && b.status === "requires-action";
  const messages = state.messages.map((m) =>
    m.blocks.some(actionable)
      ? {
          ...m,
          blocks: m.blocks.map((b) =>
            (b.kind === "approval" || b.kind === "question") && b.status === "requires-action"
              ? { ...b, status: "incomplete" as const }
              : b,
          ),
        }
      : m,
  );
  return { ...state, messages, openInterrupts: [] };
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
 *  reconciles the optimistic placeholder so the streamed item doesn't double.
 *
 *  The text block is always `complete`: a userMessage is atomic (its content
 *  is the whole prompt, present in full on both the started and completed
 *  events — never delta-streamed), so it has no "running" phase. Stamping it
 *  from item.status would make the live path (started=running → "running",
 *  then the completed event de-dupes and never upgrades it) disagree with
 *  history replay (completed-only → "complete"). Pinning "complete" keeps the
 *  two convergent. */
export function appendUserMessage(
  state: AgentViewState,
  item: ItemOf<"userMessage">,
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
      isLocalMessageId(m.id) &&
      m.blocks.find((b): b is Extract<ContentBlock, { kind: "text" }> => b.kind === "text")
        ?.text === text,
  );
  if (placeholder !== -1) {
    const messages = state.messages.map((m, i) =>
      i === placeholder ? { ...m, id: item.id, runId: item.runId } : m,
    );
    return { ...state, messages, turnMessageId: null };
  }
  const msg: Message = {
    id: item.id,
    role: "user",
    who: nameForRole("user"),
    time: formatTime(item.createdAt),
    runId: item.runId,
    blocks: [{ kind: "text", text, status: "complete" }],
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
    // Never let an empty completed snapshot wipe already-streamed text: the
    // contract is that completed restates the full content, but a malformed /
    // empty terminal frame must not blank the bubble. Keep the prior text when
    // the projection is empty (status still upgrades).
    (b) => (b.kind === "text" ? { ...b, text: text || b.text, status } : b),
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
    // Preserve already-streamed reasoning when a completed snapshot is empty
    // (see foldText) — the status still upgrades.
    (b) => (b.kind === "reasoning" ? { ...b, text: text || b.text, status } : b),
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
    name: item.tool?.name ?? "tool",
    fn: toolLabel(item.tool),
    // Tool args are authoritative from the structured Item — tools are
    // call-and-result: the runtime parses the args whole before emitting the
    // card and re-sends them on the completed Item. So at the TERMINAL state we
    // derive args from that object (argsText), which makes live streaming and
    // history replay (item.completed-only) converge. While the item is still
    // running we instead show the accumulated toolArguments-delta preview —
    // kept for a future where args stream incrementally for live UX (see
    // onItemDelta) — and the completed Item then reconciles to the object.
    args:
      item.status === "running" ? (prev?.args ?? "") || argsText(item.tool) : argsText(item.tool),
    status: toolStatus(item),
    duration,
    // Keep the accumulated stream preview as the baseline; toolFields then
    // reconciles `result` to the authoritative value once the completed Item
    // carries it (command result.output / generic tool.result). While the
    // item is still running neither is present, so the toolOutput-delta
    // accumulation stands (API.md §4.4.1 + §5.2).
    result: prev?.result,
    // Surface the tool-level failure reason (§8.1 channel b) so an "err" tool
    // tells the user *why*, not just that it went red.
    error: item.error ? (item.error.detail ?? item.error.type) : undefined,
    ...toolFields(item.tool),
  };
  return { state: { ...withBlock, toolCalls: { ...withBlock.toolCalls, [item.id]: tool } }, tool };
}
