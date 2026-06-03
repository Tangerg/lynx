// Pure (state, …) → state helpers shared by the v2 Item-fold handlers.
// No I/O — just immutable updates over AgentViewState + small wire→view
// projections (Item → message bubble + content blocks).

import type {
  ContentBlock as WireContentBlock,
  Item,
  ItemStatus,
  PlanStep,
  Question,
  ToolInvocation,
} from "@/rpc";
import type {
  AgentViewState,
  BlockStatus,
  ContentBlock,
  Message,
  MessageRole,
  PlanItem,
  QuestionItem,
  ToolCall,
  ToolCallStatus,
} from "@/protocol/run/viewState";

// ---------------------------------------------------------------------------
// Formatting / naming
// ---------------------------------------------------------------------------

function formatTime(iso?: string): string {
  const d = iso ? new Date(iso) : new Date();
  const safe = Number.isNaN(d.getTime()) ? new Date() : d;
  const h = safe.getHours() % 12 || 12;
  const m = String(safe.getMinutes()).padStart(2, "0");
  return `${h}:${m} ${safe.getHours() >= 12 ? "PM" : "AM"}`;
}

const ROLE_DISPLAY_NAME: Record<MessageRole, string> = {
  user: "You",
  assistant: "Assistant",
  system: "System",
};
function nameForRole(role: MessageRole): string {
  return ROLE_DISPLAY_NAME[role];
}

export function blockStatus(status: ItemStatus): BlockStatus {
  if (status === "inProgress") return "running";
  if (status === "incomplete") return "incomplete";
  return "complete";
}

// ---------------------------------------------------------------------------
// Wire Item → view projections
// ---------------------------------------------------------------------------

function contentText(blocks: WireContentBlock[]): string {
  return blocks
    .filter((b): b is Extract<WireContentBlock, { type: "text" }> => b.type === "text")
    .map((b) => b.text)
    .join("");
}

const PLAN_STATUS: Record<PlanStep["status"], PlanItem["status"]> = {
  completed: "done",
  inProgress: "doing",
  pending: "todo",
  failed: "todo",
};
export function mapPlan(steps: PlanStep[]): PlanItem[] {
  return steps.map((s, i) => ({
    id: i + 1,
    pid: s.id,
    status: PLAN_STATUS[s.status],
    text: s.title,
  }));
}

function mapQuestion(q: Question): QuestionItem[] {
  return q.fields.map((f) =>
    f.type === "choice"
      ? {
          id: f.name,
          question: f.label || q.prompt,
          header: f.header ?? "",
          options: f.options.map((o) => ({
            label: o.label,
            description: o.description ?? "",
            preview: o.preview,
          })),
          multiSelect: !!f.multiple,
          allowFreeText: false,
        }
      : {
          id: f.name,
          question: f.label || q.prompt,
          header: f.header ?? "",
          options: [],
          multiSelect: false,
          allowFreeText: true,
        },
  );
}

/** Human-readable label for a tool invocation (the toolCall row title). */
function toolLabel(tool: ToolInvocation): string {
  switch (tool.kind) {
    case "command":
      return tool.command;
    case "fileEdit":
      return tool.path;
    case "mcp":
      return `${tool.server}.${tool.name}`;
    case "search":
      return tool.query;
    case "subagent":
      return tool.name ?? "subagent";
  }
}

/** Derive view ToolCall fields from a (possibly completed) toolCall Item. */
function toolFields(tool: ToolInvocation): Partial<ToolCall> {
  switch (tool.kind) {
    case "command":
      return { result: tool.output };
    case "fileEdit": {
      const rows = tool.diff ?? [];
      return {
        added: rows.filter((r) => r.type === "added").length,
        removed: rows.filter((r) => r.type === "deleted").length,
      };
    }
    case "mcp":
      return { result: tool.result === undefined ? undefined : JSON.stringify(tool.result) };
    case "search":
      return { hits: tool.results?.length };
    case "subagent":
      return { result: tool.result };
  }
}

function toolStatus(item: Extract<Item, { type: "toolCall" }>): ToolCallStatus {
  if (item.error || item.status === "incomplete") return "err";
  if (item.status === "inProgress") return "running";
  return "ok";
}

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
 *  a re-seen id is a no-op, dodging React's duplicate-key warning. */
export function appendUserMessage(
  state: AgentViewState,
  item: ItemOf<"userMessage">,
  status: BlockStatus,
): AgentViewState {
  if (state.messages.some((m) => m.id === item.id)) return state;
  const msg: Message = {
    id: item.id,
    role: "user",
    who: nameForRole("user"),
    time: formatTime(item.createdAt),
    blocks: [{ kind: "text", text: contentText(item.content), status }],
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
  return upsertBlock(
    state,
    item.id,
    (b) => b.kind === "reasoning" && b.reasoningId === item.id,
    () => ({ kind: "reasoning", reasoningId: item.id, text: item.text, status }),
    (b) => (b.kind === "reasoning" ? { ...b, text: item.text, status } : b),
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
    fn: toolLabel(item.tool),
    args: prev?.args ?? "",
    status: toolStatus(item),
    duration,
    ...toolFields(item.tool),
  };
  return { state: { ...withBlock, toolCalls: { ...withBlock.toolCalls, [item.id]: tool } }, tool };
}
