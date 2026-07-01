// Pure wire → view projections + formatting. No AgentViewState here — these
// map a v2 Item (or its pieces) into the shapes the chat UI renders. The
// stateful folds that place these into AgentViewState live in `fold.ts`.

import type { DiffRow, Item, ItemStatus, PlanStep, Question, ToolInvocation } from "@/rpc";
import type { ContentBlock as WireContentBlock } from "@/rpc";
import type {
  BlockStatus,
  ContentBlock,
  MessageRole,
  PlanItem,
  QuestionItem,
  ToolCall,
  ToolCallStatus,
} from "@/plugins/builtin/agent/public/viewState";
import { toolCategory } from "@/plugins/builtin/agent/public/viewState";

// Formatting / naming

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
  if (status === "running") return "running";
  if (status === "incomplete") return "incomplete";
  return "complete";
}

// Wire Item → view projections

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

// Project a userMessage's wire content into view blocks: the merged text (one
// block) followed by one image block per inlined image (MULTIMODAL_IMAGE_INPUT,
// §4.3). A userMessage is atomic, so the text block is always `complete`.
export function userContentBlocks(content: WireContentBlock[] | undefined): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  const text = contentText(content);
  if (text) blocks.push({ kind: "text", text, status: "complete" });
  for (const b of content ?? []) {
    if (b.type === "image") blocks.push({ kind: "image", mime: b.mime, data: b.data });
  }
  return blocks;
}

const PLAN_STATUS: Record<PlanStep["status"], PlanItem["status"]> = {
  completed: "done",
  running: "doing",
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

// §4.4.2 display conventions — read the domain-neutral { name, arguments,
// result } envelope into view fields. NOT wire-enforced: unknown names fall to
// the JSON-tree generic path. The category map lives in viewState
// (`toolCategory`) so the fold, runDigest, and icon routing share one table.
// All readers are defensive: the item.started shell has no `result` and may
// have empty `arguments`, so every access tolerates absent/malformed values
// (a throw here is swallowed by the reducer's try/catch and silently drops the
// block — or strands a HITL approval the user can no longer act on).

function asRecord(v: unknown): Record<string, unknown> | undefined {
  return typeof v === "object" && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : undefined;
}
function asString(v: unknown): string | undefined {
  return typeof v === "string" ? v : undefined;
}
function asArrayLength(v: unknown): number | undefined {
  return Array.isArray(v) ? v.length : undefined;
}

/** result.changes (FileEdit[]) → the call-scoped diff rows + their +added /
 *  −removed line counts (§4.4.2 edit / §12.1 C). An `edit` now ships actual
 *  per-file `diff` rows (tooldisplay.go editDiffRows), so the card renders THIS
 *  edit's patch inline and shows real counts; a `write` (or any shape without
 *  `diff` rows) carries none, so we return {} rather than a fabricated "+0 −0"
 *  on every card (ToolMeta renders `+{added}` whenever `added != null`). */
function editLineCounts(result: unknown): Partial<ToolCall> {
  const changes = asRecord(result)?.changes;
  if (!Array.isArray(changes)) return {};
  const rows = changes.flatMap((c) => {
    const diff = asRecord(c)?.diff;
    return Array.isArray(diff) ? diff : [];
  });
  if (rows.length === 0) return {}; // {path,status} entries, no diff rows → nothing to count
  const isType = (r: unknown, t: string) => asRecord(r)?.type === t;
  return {
    diff: rows as DiffRow[],
    added: rows.filter((r) => isType(r, "added")).length,
    removed: rows.filter((r) => isType(r, "deleted")).length,
  };
}

/** First line of a free-form prompt, for row titles. */
function firstLine(v: unknown): string | undefined {
  const s = asString(v)?.trim();
  return s ? s.split("\n", 1)[0] : undefined;
}

/** Name-keyed labels for the runtime's specialised tools — these don't fit a
 *  category (each reads a different key argument). Checked BEFORE the
 *  category switch in toolLabel. */
function nameLabel(tool: ToolInvocation): string | undefined {
  const a = tool.arguments ?? {};
  switch (tool.name) {
    case "lsp": {
      // One operation-dispatched tool: operation + file_path/line/character,
      // or query for workspace_symbols (backend internal/kernel/toolset/lsptools).
      const op = asString(a.operation);
      if (op === "workspace_symbols") return asString(a.query);
      const file = asString(a.file_path);
      if (op === "document_symbols") return file;
      return file ? `${file}:${a.line ?? "?"}:${a.character ?? "?"}` : undefined;
    }
    case "lsp_diagnostics":
      return asString(a.file_path);
    case "skill": {
      const op = asString(a.op);
      const name = asString(a.name);
      return op ? (name ? `${op} ${name}` : op) : undefined;
    }
    case "ask_user": {
      // Structured questions[] — label off the first question's text.
      const first = Array.isArray(a.questions) ? asRecord(a.questions[0]) : undefined;
      return firstLine(first?.question);
    }
    case "shell_output":
    case "shell_kill":
      return asString(a.shell_id);
    default:
      return undefined;
  }
}

/** Human-readable label for a tool invocation (the toolCall row title).
 *  `undefined` on a body-less toolCall started shell (see `mapPlan`). */
export function toolLabel(tool: ToolInvocation | undefined): string {
  if (!tool) return "tool";
  const byName = nameLabel(tool);
  if (byName) return byName;
  const a = tool.arguments ?? {};
  switch (toolCategory(tool.name)) {
    case "command":
      return asString(a.command) || tool.name || "command";
    case "fileEdit": {
      const path = asString(a.file_path) ?? asString(a.path);
      if (path) return path;
      const rawChanges = asRecord(tool.result)?.changes;
      const changes = Array.isArray(rawChanges) ? rawChanges : [];
      return changes.length === 1
        ? (asString(asRecord(changes[0])?.path) ?? "file")
        : `${changes.length} files`;
    }
    case "search":
      return asString(a.query) || asString(a.pattern) || "search";
    case "webSearch":
      return asString(a.query) || "search";
    case "read":
      return asString(a.file_path) || asString(a.path) || tool.name;
    case "subagent":
      return firstLine(a.prompt) || firstLine(a.task) || tool.name;
    default:
      return tool.name || "tool";
  }
}

/** Derive view ToolCall fields from a (possibly completed) toolCall Item.
 *  `undefined` on a body-less toolCall started shell (see `mapPlan`). */
export function toolFields(tool: ToolInvocation | undefined): Partial<ToolCall> {
  if (!tool) return {};
  const result = asRecord(tool.result);
  switch (toolCategory(tool.name)) {
    case "command": {
      // The authoritative output lands on the result at item.completed
      // (durable) — surface it as the view `result` so history replay
      // (items.list → completed only, no deltas), reconnect, and
      // non-streaming runtimes all render it (API.md §5.2 / §4.4.2). The
      // item.delta{toolOutput} stream is only a live preview accumulating
      // into `result` while running; absent output here (the started shell)
      // omits the key so that preview stands until completed reconciles it.
      // Three wire dialects: the §4.4.2 convention `{exitCode, output}`, the
      // runtime's raw shell response `{exit_code, stdout, stderr}` — stderr
      // appended after stdout so failures aren't silently blank — and the
      // plain-string ack of run_in_background ("Started background shell …").
      const stdout = asString(result?.stdout);
      const stderr = asString(result?.stderr);
      const merged =
        asString(result?.output) ??
        (stdout !== undefined || stderr !== undefined
          ? [stdout, stderr].filter(Boolean).join("\n")
          : asString(tool.result));
      const exitCode = [result?.exitCode, result?.exit_code].find((v) => typeof v === "number");
      return {
        exitCode: exitCode as number | undefined,
        ...(merged !== undefined
          ? { result: merged, outputTruncated: result?.outputTruncated === true }
          : {}),
      };
    }
    case "fileEdit":
      return editLineCounts(tool.result);
    case "search":
      // grep returns ONE of matches/files/counts (output_mode); glob returns
      // paths. `hits` (§4.4.2) kept first for convention-shaped runtimes.
      // The raw result rides along so the grep/glob previews can render the
      // call's own rows instead of re-querying.
      return {
        hits:
          asArrayLength(result?.hits) ??
          asArrayLength(result?.matches) ??
          asArrayLength(result?.files) ??
          asArrayLength(result?.counts) ??
          asArrayLength(result?.paths),
        ...(tool.result !== undefined
          ? {
              result: typeof tool.result === "string" ? tool.result : JSON.stringify(tool.result),
            }
          : {}),
      };
    case "webSearch":
      // Carry the raw result alongside the hit count so the web_search preview
      // can render the result cards (same passthrough as grep/glob above).
      return {
        hits: asArrayLength(result?.results),
        ...(tool.result !== undefined
          ? { result: typeof tool.result === "string" ? tool.result : JSON.stringify(tool.result) }
          : {}),
      };
    case "read": {
      // ReadResponse carries the text on `content` — pass it through as the
      // result body (the JSON-stringified envelope is escaped noise). Omit the
      // key when absent so a completed Item without it doesn't clobber the
      // toolOutput-delta preview (same guard as command / search).
      const content = asString(result?.content);
      return content !== undefined ? { result: content } : {};
    }
    default:
      // Best-effort JSON result → a pretty string the inspector renders as a
      // JSON tree (formatBody re-parses); plain strings pass through. Omit the
      // key when absent so a completed Item without `result` doesn't clobber the
      // toolOutput-delta preview accumulated while running.
      return tool.result === undefined
        ? {}
        : {
            result:
              typeof tool.result === "string" ? tool.result : JSON.stringify(tool.result, null, 2),
          };
  }
}

/** Fallback args text when no `toolArguments` deltas streamed: the parsed
 *  `arguments`, pretty-printed (the inspector re-renders it as a JSON tree).
 *  "" for tools whose key arg is already baked into `fn` — the category ones
 *  (command / fileEdit / search / webSearch / read) and the name-labelled
 *  ones (lsp_* / skill / ask_user, see nameLabel) — and for an empty object,
 *  so a started shell seeds "" for delta accrual rather than "{}". Guards the
 *  case where a tool delivers its args only on item.completed (no streaming). */
export function argsText(tool: ToolInvocation | undefined): string {
  if (!tool) return "";
  if (nameLabel(tool) !== undefined) return "";
  if (toolCategory(tool.name) !== "generic" && toolCategory(tool.name) !== "subagent") return "";
  return Object.keys(tool.arguments ?? {}).length > 0
    ? JSON.stringify(tool.arguments, null, 2)
    : "";
}

export function toolStatus(item: Extract<Item, { type: "toolCall" }>): ToolCallStatus {
  // A HITL-declined tool settles as incomplete + error.type "denied_by_user"
  // (API.md §8.1) — that's a user decision, render it neutral, not failure-red.
  if (item.error?.type === "denied_by_user") return "denied";
  if (item.error || item.status === "incomplete") return "err";
  if (item.status === "running") return "running";
  return "ok";
}

// Approval-card projections — read the same ToolInvocation envelope the HITL
// interrupt carries (API.md §4.8). Co-located with the other tool readers
// (toolLabel / toolFields) so every `toolCategory` switch lives here, not in
// the StreamEvent dispatcher (handlers.ts).

/** Short verb phrase for an approval card title, derived from the tool category
 *  (§4.4.2 display convention). The approval payload's tool has no `result`
 *  yet, so the label keys on `name` only. */
export function approvalText(tool: ToolInvocation): string {
  switch (toolCategory(tool.name)) {
    case "command":
      return "Run command";
    case "fileEdit":
      return "Apply file change";
    case "search":
      return "Run search";
    case "webSearch":
      return "Run web search";
    case "subagent":
      return "Delegate to sub-agent";
    default:
      return `Run ${tool.name}`;
  }
}

/** The bare command string for a command-category approval (the `$ cmd` line). */
export function commandString(tool: ToolInvocation): string {
  const c = tool.arguments?.command;
  return typeof c === "string" ? c : "";
}

/** Editable args make sense for free-form tools (the JSON-tree generic envelope
 *  + subagent) — approve-with-modified-args (§6.1 editedArgs). Commands / file
 *  edits / searches bake their key arg into the card title, so no arg editor. */
export function editableArgs(tool: ToolInvocation): Record<string, unknown> | undefined {
  const cat = toolCategory(tool.name);
  return cat === "generic" || cat === "subagent" ? tool.arguments : undefined;
}
