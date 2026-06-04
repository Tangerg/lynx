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
    case "commandExecution":
      return tool.command.join(" ") || "command";
    case "fileChange":
      return tool.changes.length === 1
        ? (tool.changes[0]?.path ?? "file")
        : `${tool.changes.length} files`;
    case "search":
    case "webSearch":
      return tool.query || "search";
    case "tool":
      return tool.name;
  }
}

/** Derive view ToolCall fields from a (possibly completed) toolCall Item.
 *  `undefined` on a body-less toolCall started shell (see `mapPlan`). */
export function toolFields(tool: ToolInvocation | undefined): Partial<ToolCall> {
  if (!tool) return {};
  switch (tool.kind) {
    case "commandExecution":
      // stdout streams via item.delta{toolOutput} and accumulates into the
      // view `result`; nothing structured to override here.
      return {};
    case "fileChange": {
      const rows = tool.changes.flatMap((c) => c.diff ?? []);
      return {
        added: rows.filter((r) => r.type === "added").length,
        removed: rows.filter((r) => r.type === "deleted").length,
      };
    }
    case "search":
    case "webSearch":
      return { hits: tool.results?.length };
    case "tool":
      // Best-effort JSON result → a pretty string the inspector renders as a
      // JSON tree (formatBody re-parses); plain strings pass through.
      return {
        result:
          tool.result === undefined
            ? undefined
            : typeof tool.result === "string"
              ? tool.result
              : JSON.stringify(tool.result, null, 2),
      };
  }
}

/** Fallback args text when no `toolArguments` deltas streamed: the generic
 *  `tool`'s parsed `arguments`, pretty-printed (the inspector re-renders it as
 *  a JSON tree). "" for typed variants (their data shows via fn / added / hits)
 *  and for an empty object — so a started shell still seeds "" for delta
 *  accrual rather than "{}". Guards the case where a tool delivers its args
 *  only as an object on item.completed (no streaming). */
export function argsText(tool: ToolInvocation | undefined): string {
  if (tool?.kind === "tool" && Object.keys(tool.arguments).length > 0) {
    return JSON.stringify(tool.arguments, null, 2);
  }
  return "";
}

export function toolStatus(item: Extract<Item, { type: "toolCall" }>): ToolCallStatus {
  // A HITL-declined tool settles as incomplete + error.type "denied_by_user"
  // (API.md §8.1) — that's a user decision, render it neutral, not failure-red.
  if (item.error?.type === "denied_by_user") return "denied";
  if (item.error || item.status === "incomplete") return "err";
  if (item.status === "inProgress") return "running";
  return "ok";
}
