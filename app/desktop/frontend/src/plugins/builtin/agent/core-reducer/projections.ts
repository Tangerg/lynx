// Pure wire → view projections + formatting. No AgentViewState here — these
// map a v2 Item (or its pieces) into the shapes the chat UI renders. The
// stateful folds that place these into AgentViewState live in `fold.ts`.

import type { Item, ItemStatus, PlanStep, Question, ToolInvocation } from "@/rpc";
import type { ContentBlock as WireContentBlock } from "@/rpc";
import type {
  BlockStatus,
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

// `blocks` is absent on the `item.started` shell of a message item — its
// content streams in via item.delta and only lands whole on item.completed.
// Treat a missing/empty content as "" so the started shell folds to an empty
// text block that deltas then patch (not a crash that skips streaming).
export function contentText(blocks: WireContentBlock[] | undefined): string {
  return (blocks ?? [])
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
// Like `contentText`, tolerate a body-less started shell: the `steps` /
// `question` / `tool` fields are absent on the `item.started` shell of a
// plan / question / toolCall and arrive whole on item.completed (plan/tool
// also stream via item.delta). Default the missing field so the shell folds
// to an empty block that later events patch — not a throw the reducer's
// try/catch swallows, leaving the block permanently unrendered.
export function mapPlan(steps: PlanStep[] | undefined): PlanItem[] {
  return (steps ?? []).map((s, i) => ({
    id: i + 1,
    pid: s.id,
    status: PLAN_STATUS[s.status],
    text: s.title,
  }));
}

export function mapQuestion(q: Question | undefined): QuestionItem[] {
  const prompt = q?.prompt ?? "";
  return (q?.fields ?? []).map((f) =>
    f.type === "choice"
      ? {
          id: f.name,
          question: f.label || prompt,
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
          question: f.label || prompt,
          header: f.header ?? "",
          options: [],
          multiSelect: false,
          allowFreeText: true,
        },
  );
}

/** Human-readable label for a tool invocation (the toolCall row title).
 *  `undefined` on a body-less toolCall started shell (see `mapPlan`). */
export function toolLabel(tool: ToolInvocation | undefined): string {
  if (!tool) return "tool";
  switch (tool.kind) {
    case "command":
      // The runtime puts the command in the streamed args, not `command`; use
      // the tool name ("bash") so the row isn't a nameless "()".
      return tool.command ?? tool.name ?? "command";
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

/** Derive view ToolCall fields from a (possibly completed) toolCall Item.
 *  `undefined` on a body-less toolCall started shell (see `mapPlan`). */
export function toolFields(tool: ToolInvocation | undefined): Partial<ToolCall> {
  if (!tool) return {};
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
