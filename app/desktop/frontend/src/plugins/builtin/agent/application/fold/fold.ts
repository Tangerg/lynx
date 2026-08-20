// Stateful folds — immutable (state, …) → state updates that place projected
// Items into AgentSessionView. The pure Agent-fact→view mappers they build on
// live in `projections.ts`; stream dispatch is registered at the Adapter boundary.

import type { AgentItem } from "@/plugins/sdk";
import type { BlockStatus, ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentSessionView, Message, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { isOptimisticSteerMessageId } from "../view/optimisticMessageIdentity";
import {
  argsText,
  contentText,
  mapQuestion,
  mapQuestionAnswers,
  toolFields,
  toolLabel,
  toolLabelKind,
  toolStatus,
  userContentBlocks,
} from "./projections";

// Message / block mutations

function mutateMessage(
  state: AgentSessionView,
  id: string,
  fn: (m: Message) => Message,
): AgentSessionView {
  return { ...state, messages: state.messages.map((m) => (m.id === id ? fn(m) : m)) };
}

/** Ensure an open assistant-turn message exists; return its id + next state. */
function ensureTurn(
  state: AgentSessionView,
  runId: string,
  itemId: string,
  createdAt?: string,
): { state: AgentSessionView; id: string } {
  const assistantTurnMessageId = state.assistantTurnByRunId[runId];
  const open =
    assistantTurnMessageId &&
    state.messages.some((message) => message.id === assistantTurnMessageId)
      ? assistantTurnMessageId
      : null;
  if (open) return { state, id: open };
  const id = `turn:${itemId}`;
  // The turn for THIS item may exist while not being the open one — a user
  // message (a send, or a mid-run steer) closes the turn, and a later block for
  // the same item comes back here. Re-adopt it: minting the id a second time
  // would put two messages under one React key, the loop CLAUDE.md §5 names.
  // Every other append in this fold already checks by id before pushing; this
  // was the one that only checked which turn was open.
  if (state.messages.some((m) => m.id === id)) {
    return {
      state: {
        ...state,
        assistantTurnByRunId: { ...state.assistantTurnByRunId, [runId]: id },
      },
      id,
    };
  }

  // The Item that opened this turn dates it. No clock here: a turn stamped by the
  // client clock sits in a stream stamped by the runtime, and the date separator
  // above it would disagree with the messages beside it. Absent is honest.
  const msg: Message = {
    id,
    role: "assistant",
    phase: "commentary",
    createdAt,
    runId,
    blocks: [],
  };
  return {
    state: {
      ...state,
      messages: [...state.messages, msg],
      assistantTurnByRunId: { ...state.assistantTurnByRunId, [runId]: id },
    },
    id,
  };
}

/** Append a block to the current assistant turn (creating the turn if needed). */
export function appendToTurn(
  state: AgentSessionView,
  runId: string,
  itemId: string,
  block: ContentBlock,
  /** The Item's wire timestamp — dates the turn if this block has to open one. */
  createdAt?: string,
): AgentSessionView {
  const { state: next, id } = ensureTurn(state, runId, itemId, createdAt);
  return mutateMessage(next, id, (message) => ({
    ...message,
    blocks: [...message.blocks, block],
  }));
}

export function patchRunBlock(
  state: AgentSessionView,
  runId: string,
  match: (block: ContentBlock) => boolean,
  patch: (block: ContentBlock) => ContentBlock,
): AgentSessionView {
  let patched = false;
  const messages = state.messages.map((message) => {
    if (patched || message.runId !== runId || !message.blocks.some(match)) return message;
    patched = true;
    return {
      ...message,
      blocks: message.blocks.map((block) => (match(block) ? patch(block) : block)),
    };
  });
  return patched ? { ...state, messages } : state;
}

/** Upsert: patch the matching block if present, else append a fresh one to
 *  the turn. Used by item.completed handlers (item.started may fall before
 *  the replay cursor or be absent from persisted-history hydration). */
function upsertBlock(
  state: AgentSessionView,
  item: { id: string; runId: string; createdAt: string },
  match: (b: ContentBlock) => boolean,
  make: () => ContentBlock,
  patch: (b: ContentBlock) => ContentBlock,
): AgentSessionView {
  if (
    state.messages.some((message) => message.runId === item.runId && message.blocks.some(match))
  ) {
    return patchRunBlock(state, item.runId, match, patch);
  }
  return appendToTurn(state, item.runId, item.id, make(), item.createdAt);
}

export function updateTool(
  state: AgentSessionView,
  runId: string,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentSessionView {
  const existing = state.toolCalls[id];
  if (!existing || existing.runId !== runId) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

export function markToolRequiresAction(
  state: AgentSessionView,
  runId: string,
  id: string,
): AgentSessionView {
  return updateTool(state, runId, id, (tool) =>
    tool.status === "requires-action" ? tool : { ...tool, status: "requires-action" },
  );
}

/** Drop every pending interrupt and downgrade its still-actionable approval /
 *  question card to `incomplete`. Called on a terminal run end (not an
 *  interrupt): the run that owned the interrupt is finished, so a card left in
 *  `requires-action` would offer buttons that resume a dead run. No-op when
 *  nothing is pending (a resolved interrupt already emptied the list). */
export function settleRunPendingInterrupts(
  state: AgentSessionView,
  runId: string,
): AgentSessionView {
  const owned = state.pendingInterrupts.filter((group) => group.runId === runId);
  if (owned.length === 0) return state;
  const interruptItemIds = new Set(
    owned.flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId)),
  );
  const actionable = (block: ContentBlock) =>
    (block.kind === "approval" || block.kind === "question") &&
    block.status === "requires-action" &&
    block.itemId !== undefined &&
    interruptItemIds.has(block.itemId);
  const messages = state.messages.map((m) =>
    m.blocks.some(actionable)
      ? {
          ...m,
          blocks: m.blocks.map((b) =>
            actionable(b) ? { ...b, status: "incomplete" as const } : b,
          ),
        }
      : m,
  );
  let toolCalls = state.toolCalls;
  for (const id of interruptItemIds) {
    const tool = toolCalls[id];
    if (!tool || tool.status !== "requires-action") continue;
    toolCalls = { ...toolCalls, [id]: { ...tool, status: "err" } };
  }
  return {
    ...state,
    messages,
    toolCalls,
    pendingInterrupts: state.pendingInterrupts.filter((group) => group.runId !== runId),
  };
}

// Per-item folds — shared by item.started (append) and item.completed
// (upsert). started/completed differ only in the block status they stamp,
// so both call through here; the upsert keeps stream replay / persisted-history
// hydration idempotent (a re-seen item patches in place, never duplicates).

type ItemOf<T extends AgentItem["type"]> = Extract<AgentItem, { type: T }>;

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
  state: AgentSessionView,
  item: ItemOf<"userMessage">,
): AgentSessionView {
  // A runs.start acknowledgement can relabel the optimistic local bubble to
  // this durable Item id before the Item itself arrives. Re-seeing that id must
  // still attach the authoritative Run owner.
  const durable = state.messages.find((message) => message.id === item.id);
  if (durable) {
    const withOwner =
      durable.runId === item.runId
        ? state
        : {
            ...state,
            messages: state.messages.map((message) =>
              message.id === item.id ? { ...message, runId: item.runId } : message,
            ),
          };
    return closeAssistantTurn(withOwner, item.runId);
  }
  const text = contentText(item.content);
  // Reconcile the optimistic placeholder: send()/steer render the user's bubble
  // immediately with a local-* id, a round-trip before the runtime streams the
  // real userMessage Item (with its own server id). Upgrade the matching
  // placeholder's id in place rather than appending a duplicate.
  //
  // Normalize a missing text block to "" so an IMAGE-ONLY bubble (no text block,
  // e.g. paste-and-send a screenshot) reconciles against its image-only streamed
  // Item (whose contentText is also "").
  const localText = (m: Message): string =>
    m.blocks.find((b): b is Extract<ContentBlock, { kind: "text" }> => b.kind === "text")?.text ??
    "";
  // A steer ack carries no Item id, so its optimistic bubble is the only case
  // that reconciles by content. A fresh start is relabeled from the mandatory
  // userItemId in StartRunResponse before its durable Item arrives.
  const matches = (m: Message): boolean => m.role === "user" && localText(m) === text;
  const placeholder = state.messages.findIndex(
    (m) => isOptimisticSteerMessageId(m.id) && matches(m),
  );
  if (placeholder !== -1) {
    const messages = state.messages.map((m, i) =>
      i === placeholder ? { ...m, id: item.id, runId: item.runId } : m,
    );
    return closeAssistantTurn({ ...state, messages }, item.runId);
  }
  const msg: Message = {
    id: item.id,
    role: "user",
    createdAt: item.createdAt,
    runId: item.runId,
    blocks: userContentBlocks(item.content),
  };
  return closeAssistantTurn({ ...state, messages: [...state.messages, msg] }, item.runId);
}

/** Upsert the agentMessage text block for an item. */
export function foldText(
  state: AgentSessionView,
  item: ItemOf<"agentMessage">,
  status: BlockStatus,
): AgentSessionView {
  if (item.phase === "finalAnswer") return foldFinalText(state, item, status);
  const text = contentText(item.content);
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "text" && b.itemId === item.id,
    () => ({ kind: "text", itemId: item.id, text, status }),
    // Never let an empty completed snapshot wipe already-streamed text: the
    // contract is that completed restates the full content, but a malformed /
    // empty terminal frame must not blank the bubble. Keep the prior text when
    // the projection is empty (status still upgrades).
    (b) => (b.kind === "text" ? { ...b, text: text || b.text, status } : b),
  );
}

/** Rehome the authoritative final answer out of the Run's working narrative.
 *
 * item.started cannot carry a phase, so live text first streams into the same
 * commentary turn as reasoning and Tools. item.completed is the first frame that
 * can authoritatively classify it. Moving the existing block here makes live,
 * replay, and mixed hydration converge without guessing from order or wording. */
function foldFinalText(
  state: AgentSessionView,
  item: ItemOf<"agentMessage">,
  status: BlockStatus,
): AgentSessionView {
  const finalId = `final:${item.id}`;
  const projectedText = contentText(item.content);
  const previous = state.messages
    .flatMap((message) => message.blocks)
    .find(
      (block): block is Extract<ContentBlock, { kind: "text" }> =>
        block.kind === "text" && block.itemId === item.id,
    );
  const block: Extract<ContentBlock, { kind: "text" }> = {
    kind: "text",
    itemId: item.id,
    text: projectedText || previous?.text || "",
    status,
  };

  let foundFinal = false;
  const messages: Message[] = [];
  for (const message of state.messages) {
    if (message.id === finalId) {
      foundFinal = true;
      messages.push({ ...message, phase: "finalAnswer", blocks: [block] });
      continue;
    }
    if (message.runId !== item.runId) {
      messages.push(message);
      continue;
    }
    const blocks = message.blocks.filter(
      (candidate) => !(candidate.kind === "text" && candidate.itemId === item.id),
    );
    if (blocks.length === message.blocks.length) {
      messages.push(message);
    } else if (blocks.length > 0) {
      messages.push({ ...message, phase: "commentary", blocks });
    }
  }

  if (!foundFinal) {
    messages.push({
      id: finalId,
      role: "assistant",
      phase: "finalAnswer",
      createdAt: item.createdAt,
      runId: item.runId,
      blocks: [block],
    });
  }
  return closeAssistantTurn({ ...state, messages }, item.runId);
}

/** Upsert the reasoning block for an item. */
export function foldReasoning(
  state: AgentSessionView,
  item: ItemOf<"reasoning">,
  status: BlockStatus,
): AgentSessionView {
  // `text` is absent on the item.started shell — it streams via item.delta
  // (same as agentMessage content). Seed "" so deltas accumulate cleanly
  // instead of onto `undefined`.
  const text = item.text ?? "";
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "reasoning" && b.reasoningId === item.id,
    () => ({ kind: "reasoning", reasoningId: item.id, text, status }),
    // Preserve already-streamed reasoning when a completed snapshot is empty
    // (see foldText) — the status still upgrades.
    (b) => (b.kind === "reasoning" ? { ...b, text: text || b.text, status } : b),
  );
}

/** Upsert the question block for an item (only `status` changes once shown). */
export function foldQuestion(
  state: AgentSessionView,
  item: ItemOf<"question">,
  status: BlockStatus,
): AgentSessionView {
  const questions = mapQuestion(item.question);
  const answers = mapQuestionAnswers(item.question);
  return upsertBlock(
    state,
    item,
    (b) => b.kind === "question" && b.itemId === item.id,
    () => ({
      kind: "question",
      status,
      itemId: item.id,
      questions,
      answered: answers !== undefined,
      answers,
    }),
    (b) =>
      b.kind === "question"
        ? {
            ...b,
            status,
            ...(item.question ? { questions, answered: answers !== undefined, answers } : {}),
          }
        : b,
  );
}

/** Place a compaction boundary as its OWN system message — a standalone
 *  "context compacted" divider, not folded into an assistant turn (a compaction
 *  sits between turns). Idempotent by the Item id: item.started then
 *  item.completed (and stream replay / persisted-history hydration) re-see the same id
 *  and patch the existing divider in place, never appending a second. Leaves
 *  assistant-turn cursor untouched — only a userMessage is a turn boundary. */
export function foldCompaction(
  state: AgentSessionView,
  item: ItemOf<"compaction">,
): AgentSessionView {
  const block: ContentBlock = {
    kind: "compaction",
    summary: item.summary,
    droppedMessages: item.droppedMessages,
  };
  if (state.messages.some((m) => m.id === item.id)) {
    return mutateMessage(state, item.id, (m) => ({ ...m, blocks: [block] }));
  }
  const msg: Message = {
    id: item.id,
    role: "system",
    createdAt: item.createdAt,
    runId: item.runId,
    blocks: [block],
  };
  return { ...state, messages: [...state.messages, msg] };
}

/** Ensure the tool block + write its toolCalls entry; preserves any
 *  accumulated arg text. Returns the next state + the resolved ToolCall (the
 *  caller stamps the matching tool-start / tool-end timeline entry). */
export function writeToolCall(
  state: AgentSessionView,
  item: ItemOf<"toolCall">,
): { state: AgentSessionView; tool: ToolCall } {
  const withBlock =
    state.toolCalls[item.id] === undefined
      ? appendToTurn(
          state,
          item.runId,
          item.id,
          { kind: "tool", toolCallId: item.id },
          item.startedAt,
        )
      : state;
  const prev = withBlock.toolCalls[item.id];
  const tool: ToolCall = {
    id: item.id,
    runId: item.runId,
    name: item.tool?.name ?? "tool",
    fn: toolLabel(item.tool),
    ...(toolLabelKind(item.tool) === "path" ? { fnKind: "path" as const } : {}),
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
    // Keep the accumulated stream preview as the baseline; toolFields then
    // reconciles `result` to the authoritative value once the completed Item
    // carries it (command result.output / generic tool.result). While the
    // item is still running neither is present, so the toolOutput-delta
    // accumulation stands (API.md §4.4.1 + §5.2).
    result: prev?.result,
    // Surface the tool-level failure reason (§8.1 channel b) so an "err" tool
    // tells the user *why*, not just that it went red.
    error: item.error ? (item.error.message ?? item.error.code) : undefined,
    durationMillis: item.durationMillis,
    safetyClass: item.safetyClass,
    approvalDecision: item.approvalDecision,
    ...toolFields(item.tool),
  };
  return { state: { ...withBlock, toolCalls: { ...withBlock.toolCalls, [item.id]: tool } }, tool };
}

function closeAssistantTurn(state: AgentSessionView, runId: string): AgentSessionView {
  if (!(runId in state.assistantTurnByRunId)) return state;
  const assistantTurnByRunId = { ...state.assistantTurnByRunId };
  delete assistantTurnByRunId[runId];
  return { ...state, assistantTurnByRunId };
}
