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

export function formatTime(iso?: string): string {
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
export function nameForRole(role: MessageRole): string {
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

export function contentText(blocks: WireContentBlock[]): string {
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

export function mapQuestion(q: Question): QuestionItem[] {
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
export function toolLabel(tool: ToolInvocation): string {
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
export function toolFields(tool: ToolInvocation): Partial<ToolCall> {
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

export function toolStatus(item: Extract<Item, { type: "toolCall" }>): ToolCallStatus {
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
export function ensureTurn(
  state: AgentViewState,
  itemId: string,
): { state: AgentViewState; id: string } {
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
export function upsertBlock(
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
