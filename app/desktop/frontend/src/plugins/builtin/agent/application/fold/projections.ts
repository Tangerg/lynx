// Pure wire → view projections + formatting. No AgentSessionView here — these
// map a v2 Item (or its pieces) into the shapes the chat UI renders. The
// stateful folds that place these into AgentSessionView live in `fold.ts`.

import type { Item, ItemStatus, Question, ToolInvocation } from "@/rpc";
import type { ContentBlock as WireContentBlock } from "@/rpc";
import type { BlockStatus, ContentBlock, QuestionItem } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall, ToolCallStatus, ToolDiffRow } from "@/plugins/sdk/types/agentSessionView";
import { toolCategory } from "../../domain/toolCategory";

/** When an Item came into being. A toolCall spans time and names its endpoints
 *  `startedAt` / `finishedAt`; every other Item is instantaneous and carries
 *  `createdAt`. Readers that only need "when" should not have to know which. */
export function itemStartedAt(item: Item): string {
  return item.type === "toolCall" ? item.startedAt : item.createdAt;
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

// Like `contentText`, tolerate a body-less started shell: the `question` / `tool`
// fields are absent on the `item.started` shell of a question / toolCall and
// arrive whole on item.completed (tool also streams via item.delta). Default the
// missing field so the shell folds to an empty block that later events patch —
// not a throw the reducer's try/catch swallows, leaving the block permanently
// unrendered.
export function mapQuestion(q: Question | undefined): QuestionItem[] {
  return (q?.fields ?? []).map((f) =>
    f.type === "choice"
      ? {
          type: "choice" as const,
          prompt: f.prompt,
          header: f.header ?? "",
          options: f.options.map((o) => ({
            label: o.label,
            description: o.description ?? "",
            preview: o.preview,
          })),
          multiple: !!f.multiple,
          allowCustom: !!f.allowCustom,
        }
      : {
          type: "text" as const,
          prompt: f.prompt,
          header: f.header ?? "",
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
function asNumber(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}
function asArrayLength(v: unknown): number | undefined {
  return Array.isArray(v) ? v.length : undefined;
}

function toolDiffRow(value: unknown): ToolDiffRow | undefined {
  const row = asRecord(value);
  if (!row) return undefined;
  const type = asString(row.type);
  if (type === "hunk") {
    const text = asString(row.text);
    return text === undefined ? undefined : { type, text };
  }
  const code = asString(row.code);
  if (code === undefined) return undefined;
  if (type === "context") {
    const leftLine = asNumber(row.leftLine);
    const rightLine = asNumber(row.rightLine);
    return leftLine === undefined || rightLine === undefined
      ? undefined
      : { type, leftLine, rightLine, code };
  }
  if (type === "added") {
    const rightLine = asNumber(row.rightLine);
    return rightLine === undefined ? undefined : { type, rightLine, code };
  }
  if (type === "deleted") {
    const leftLine = asNumber(row.leftLine);
    return leftLine === undefined ? undefined : { type, leftLine, code };
  }
  return undefined;
}

/** result.changes (FileEdit[]) → the call-scoped diff rows + their +added /
 *  −removed line counts (§4.4.2 edit / §12.1 C). An `edit` now ships actual
 *  per-file `diff` rows (tooldisplay.go editDiffRows), so the card renders THIS
 *  edit's patch inline and shows real counts; a `write` (or any shape without
 *  `diff` rows) carries none, so we return {} rather than a fabricated "+0 −0"
 *  on every card (ToolMeta renders `+{added}` whenever `added != null`). */
/** result.changes (FileEdit[]) — the files one edit call touched. */
function editChanges(result: unknown): unknown[] {
  const changes = asRecord(result)?.changes;
  return Array.isArray(changes) ? changes : [];
}

function editLineCounts(result: unknown): Partial<ToolCall> {
  const changes = editChanges(result);
  if (changes.length === 0) return {};
  const rows = changes.flatMap((c): ToolDiffRow[] => {
    const diff = asRecord(c)?.diff;
    return Array.isArray(diff) ? diff.flatMap((row) => toolDiffRow(row) ?? []) : [];
  });
  if (rows.length === 0) return {}; // {path,status} entries, no diff rows → nothing to count
  return {
    diff: rows,
    added: rows.filter((row) => row.type === "added").length,
    removed: rows.filter((row) => row.type === "deleted").length,
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
      // One operation-dispatched tool: operation + path/line/character, or query
      // for workspace_symbols. There is no separate lsp_diagnostics — diagnostics
      // is one of this tool's operations, and the runtime asserts the two never
      // coexist.
      const op = asString(a.operation);
      if (op === "workspace_symbols") return asString(a.query);
      const path = asString(a.path);
      if (op === "document_symbols" || op === "diagnostics") return path;
      return path ? `${path}:${a.line ?? "?"}:${a.character ?? "?"}` : undefined;
    }
    case "ask_user": {
      // Structured questions[] — label off the first question's text.
      const first = Array.isArray(a.questions) ? asRecord(a.questions[0]) : undefined;
      return firstLine(first?.question);
    }
    case "read_shell_output":
    case "stop_shell":
      return asString(a.shell_id);
    case "load_skill":
    case "propose_skill":
      return asString(a.name);
    case "read_skill_resource": {
      const name = asString(a.name);
      const path = asString(a.path);
      return name && path ? `${name}/${path}` : (name ?? path);
    }
    case "search_memory":
    case "search_conversations":
    case "search_tools":
      return asString(a.query);
    case "web_fetch":
    case "http_request":
      return asString(a.url);
    default:
      return undefined;
  }
}

/** Human-readable label for a tool invocation (the toolCall row title). */
export function toolLabel(tool: ToolInvocation | undefined): string {
  if (!tool) return "tool";
  const byName = nameLabel(tool);
  if (byName) return byName;
  const a = tool.arguments ?? {};
  switch (toolCategory(tool.name)) {
    case "command":
      // `description` is required by the shell tool and is an action phrase ("Run
      // backend tests"); the command itself rides beside it as the row's mono
      // detail. A title spelled as the command line puts data in the slot meant
      // for intent, and repeats verbatim what the detail already shows.
      return asString(a.description) || asString(a.command) || tool.name || "command";
    case "fileEdit": {
      const path = asString(a.path);
      if (path) return path;
      const single = asString(asRecord(editChanges(tool.result)[0])?.path);
      // No single path to show: leave the tool's own name, which is how every
      // other label-less case reports "I have nothing better" — presentation
      // then resolves it through TOOL_LABEL_KEYS, and the file count rides along
      // as a meta chip. A fold that spelled "3 files" here froze one language
      // into the view state.
      return single ?? tool.name;
    }
    case "search":
      return asString(a.query) || asString(a.pattern) || "search";
    case "webSearch":
      return asString(a.query) || "search";
    case "read":
      return asString(a.path) || tool.name;
    case "subagent":
      // delegate_task requires a 3-5 word `summary` precisely so the parent row can
      // name the delegated work without quoting the whole brief.
      return asString(a.summary) || firstLine(a.instructions) || tool.name;
    default:
      return tool.name || "tool";
  }
}

/** Derive view ToolCall fields from a (possibly completed) toolCall Item. */
export function toolFields(tool: ToolInvocation | undefined): Partial<ToolCall> {
  if (!tool) return {};
  const result = asRecord(tool.result);
  const operation = asString(tool.arguments?.operation);
  return {
    ...(operation !== undefined ? { operation } : {}),
    ...categoryFields(tool, result),
  };
}

function categoryFields(
  tool: ToolInvocation,
  result: Record<string, unknown> | undefined,
): Partial<ToolCall> {
  switch (toolCategory(tool.name)) {
    case "command": {
      // The authoritative, persisted output lands on the result at item.completed
      // — surface it as the view `result` so history hydration
      // (items.list → completed only, no deltas), reconnect, and
      // non-streaming runtimes all render it (API.md §5.2 / §4.4.2). The
      // item.delta{toolOutput} stream is only a live preview accumulating
      // into `result` while running; absent output here (the started shell)
      // omits the key so that preview stands until completed reconciles it.
      //
      // Two shapes, not three: the runtime's presenter merges stdout/stderr and
      // projects every shell result to `{ output, exitCode }` before it reaches
      // the wire, so the raw `{stdout, stderr, exit_code}` dialect this used to
      // also accept can no longer arrive. What remains is that envelope and the
      // plain-string ack of run_in_background ("Started background shell …").
      const merged = asString(result?.output) ?? asString(tool.result);
      return {
        exitCode: asNumber(result?.exitCode),
        // The command itself: a row titles itself with the human `description`
        // now, and this is the line the reader actually verifies. It is not in
        // `args` — a command call bakes its key argument into the label rather
        // than streaming arg text.
        ...(asString(tool.arguments?.command) !== undefined
          ? { command: asString(tool.arguments?.command) }
          : {}),
        ...(merged !== undefined ? { result: merged } : {}),
      };
    }
    case "fileEdit": {
      const changes = editChanges(tool.result);
      return {
        ...editLineCounts(tool.result),
        ...(changes.length > 1 ? { files: changes.length } : {}),
      };
    }
    case "search":
      // The runtime's presenter folds grep's matches/files/counts and glob's paths
      // into one `hits: [{path, snippet?, lineNumber?}]` envelope, so this reads the
      // single projected shape. The raw result rides along so the grep/glob previews
      // can render the call's own rows instead of re-querying.
      return {
        hits: asArrayLength(result?.hits),
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
      const lines = asNumber(result?.total_lines);
      return {
        ...(content !== undefined ? { result: content } : {}),
        ...(lines !== undefined ? { lines } : {}),
      };
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
  // (API.md §8.1). A user's decision is not a fault, so it is its own status rather
  // than an error — the card reads it as warning-toned, never failure-red.
  if (item.error?.type === "denied_by_user") return "denied";
  if (item.error || item.status === "incomplete") return "err";
  if (item.status === "running") return "running";
  return "ok";
}

// Approval-card projections — read the same ToolInvocation envelope the HITL
// interrupt carries (API.md §4.8). Co-located with the other tool readers
// (toolLabel / toolFields) so every `toolCategory` switch lives here, not in
// the StreamEvent dispatcher (handlers.ts).

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
