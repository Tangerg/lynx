import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ActivityShell } from "@/lib/activityShell";
import { fmtDuration } from "@/lib/format";

/**
 * The thing a call acted on, and what kind of thing it is.
 *
 * Typed rather than a bare string because a path is not a string with slashes in
 * it: it truncates from the other end (see the FilePath atom), and only this layer
 * knows which of a call's arguments it read. Handing the renderer a string threw
 * that knowledge away and left it guessing — which it did by not guessing, so
 * every path lost its filename to an ellipsis.
 */
export type ToolDetail = { kind: "path" | "text"; value: string };

export interface ToolIntent {
  /** What the row calls the call. Also a value with a nature: for the file
   *  categories the projection puts the path here rather than in `detail`, so this
   *  is the slot that has to keep the filename. */
  label: ToolDetail;
  detail?: ToolDetail;
}

export type ToolMetaTone = "success" | "negative" | "muted";

export interface ToolMetaItem {
  id: string;
  label: string;
  tone: ToolMetaTone;
}

// Catalog keys, not words. A tool row's title is either one of these (a verb the
// UI chose) or the runtime's own tool name, which is data and stays verbatim —
// that mix is why the translator arrives as an argument here instead of the ring
// returning `labelKey` the way the run-summary view model can.
const TOOL_LABEL_KEYS: Record<string, string> = {
  shell: "tool.label.shell",
  read: "tool.label.read",
  edit: "tool.label.edit",
  write: "tool.label.write",
  apply_patch: "tool.label.applyPatch",
  grep: "tool.label.grep",
  glob: "tool.label.glob",
  lsp: "tool.label.lsp",
  // The Plan-mode trio and the catalog reads have no identifying argument to put
  // in a title, so without an entry here a row reads as the raw snake_case name.
  enter_plan_mode: "tool.label.enterPlanMode",
  set_plan: "tool.label.setPlan",
  exit_plan_mode: "tool.label.exitPlanMode",
  list_skills: "tool.label.listSkills",
  list_schedules: "tool.label.listSchedules",
  read_tool_result: "tool.label.readToolResult",
};

// Which argument names an identifying subject, and how that subject reads. `path`
// is the runtime's own spelling for it (Semantics.ApprovalSubject reads the same
// field), so the two cannot drift apart on a rename without the approval rules
// drifting too.
const TOOL_DETAIL_KEYS: ReadonlyArray<{ key: string; kind: ToolDetail["kind"] }> = [
  { key: "path", kind: "path" },
  { key: "query", kind: "text" },
  { key: "pattern", kind: "text" },
  { key: "url", kind: "text" },
];

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const parsed = parseToolArgs(tool.args);
  // Only when the projection had nothing better than the tool's own name to show
  // (`fn` is normally the command or the path it acted on). Keyed on that
  // condition rather than on `fn` matching a name in the table, which is the same
  // thing by coincidence until a shell command happens to be spelled `grep`.
  const labelKey = tool.fn === tool.name ? TOOL_LABEL_KEYS[tool.name] : undefined;
  const label: ToolDetail = labelKey
    ? { kind: "text", value: t(labelKey) }
    : { kind: tool.fnKind ?? "text", value: tool.fn };
  // In order of how directly each answers "what did this do": the command a
  // shell ran, the step a plan is on, else whatever identifying argument the
  // call's arg text carries. The first two are facts the fold lifted out of the
  // arguments; only the last has to be looked for.
  const detail = text(tool.command) ?? text(tool.step) ?? (parsed ? toolDetail(parsed) : undefined);
  // A row has two slots for one subject, and where the title already IS the
  // subject the second one has nothing left to say. The fold titles a command
  // with its `description` and keeps the command line for this slot, but
  // `description` is the tool's contract rather than the wire's guarantee — with
  // it absent the title falls back to the command, and both slots then printed the
  // same shell line, the first of them truncated at a different width.
  return detail && detail.value === label.value ? { label } : { label, detail };
}

export function toolMetaItems(t: Translate, tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  // Added and removed are NOT chips: a diffstat is one fact with two numbers, and
  // the app already renders it as `+n −m` in the diff header and the run summary.
  // Two chips worded in prose made the same fact look like two, and made it look
  // different here than everywhere else it appears. See `toolDiffStat`.
  // Notation, not words: a ratio and a line span read the same in every language,
  // and a catalog entry that holds no word gives a translator nothing to do while
  // making the format harder to find. (The two diffstat entries this replaced were
  // exactly that — "+{{count}}" and "-{{count}}" filed under translation.)
  if (tool.progress != null) {
    items.push({
      id: "progress",
      label: `${tool.progress.done}/${tool.progress.total}`,
      tone: "muted",
    });
  }
  if (tool.files != null) {
    items.push({ id: "files", label: t("tool.meta.files", { count: tool.files }), tone: "muted" });
  }
  if (tool.hits != null) {
    items.push({ id: "hits", label: t("tool.meta.matches", { count: tool.hits }), tone: "muted" });
  }
  if (tool.range != null) {
    items.push({ id: "range", label: `L${tool.range.start}-${tool.range.end}`, tone: "muted" });
  }
  if (tool.lines != null) {
    items.push({ id: "lines", label: t("tool.meta.lines", { count: tool.lines }), tone: "muted" });
  }
  if (tool.exitCode != null && tool.exitCode !== 0) {
    items.push({
      id: "exit",
      label: t("tool.meta.exit", { code: tool.exitCode }),
      tone: "negative",
    });
  }
  // The runtime measures this; a client stopwatch would time its own render loop.
  // Sub-second calls are omitted: "0.1s" on a dozen reads is noise, and the number
  // only earns its place once it answers "why did that take so long".
  if (tool.durationMillis != null && tool.durationMillis >= 1000) {
    items.push({ id: "duration", label: fmtDuration(tool.durationMillis), tone: "muted" });
  }
  if (tool.status === "running") {
    items.push({ id: "live", label: t("tool.meta.live"), tone: "muted" });
  }
  return items;
}

/**
 * The call's diffstat, when it has one worth showing.
 *
 * Absent rather than zeroed when nothing was measured: the atom draws a dash to
 * hold a column in the diff views, and a dash on a transcript row is a mark the
 * reader has to stop and interpret. A no-op edit reports nothing instead.
 */
export function toolDiffStat(tool: ToolCall): { added: number; removed: number } | undefined {
  const added = tool.added ?? 0;
  const removed = tool.removed ?? 0;
  if (tool.added == null && tool.removed == null) return undefined;
  if (added === 0 && removed === 0) return undefined;
  return { added, removed };
}

/**
 * Whether a call had no side effect the user would need to approve.
 *
 * The runtime's own answer, stamped on the toolCall Item — the same table the
 * approval gate consults, so the row's weight and the gate's decision cannot
 * disagree. A presentation-side list would duplicate protocol policy and become
 * stale when tools change.
 */
export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

/**
 * How much of the plane one tool invocation claims.
 *
 * Codex keeps the invocation on the transparent work-narrative plane regardless
 * of safety class or outcome. A command output, diff, or other material result
 * earns a surface only inside the disclosed body. Keeping that boundary here
 * prevents new tools and new lifecycle states from silently reintroducing the
 * dashboard-like stack of tinted cards.
 */
export function toolActivityShell(_tool: ToolCall): ActivityShell {
  return "line";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

/**
 * What a run of calls DID, counted per kind of act.
 *
 * The families below are the runtime's own safety classes plus the two reads worth
 * telling apart (a file and a symbol), so a tool added or renamed on the backend
 * lands in the right one with no table here to update. Order is fixed rather than
 * by-count: a row that reorders itself as counts change is a row a reader has to
 * re-read every time.
 *
 * Both callers show it in the LABEL and keep a total in the meta column — the
 * summary says what happened, the total says how much is behind the row. The
 * total includes conclusions that cannot be classified as calls.
 */
const ACTIVITY_FAMILIES = ["read", "search", "lookup", "write", "run", "fetch"] as const;

type ActivityFamily = (typeof ACTIVITY_FAMILIES)[number];

function activityFamily(tool: ToolCall): ActivityFamily {
  if (tool.name === "read") return "read";
  if (tool.name === "lsp") return "lookup";
  if (tool.safetyClass === "write") return "write";
  if (tool.safetyClass === "exec") return "run";
  if (tool.safetyClass === "network") return "fetch";
  return "search";
}

export function summarizeActivity(t: Translate, tools: readonly ToolCall[]): string {
  const counts = new Map<ActivityFamily, number>();
  for (const tool of tools) {
    const family = activityFamily(tool);
    counts.set(family, (counts.get(family) ?? 0) + 1);
  }

  const parts: string[] = [];
  for (const family of ACTIVITY_FAMILIES) {
    const count = counts.get(family);
    if (count) parts.push(t(`tool.group.${family}`, { count }));
  }
  return parts.join(" · ");
}

function parseToolArgs(args: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(args || "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function text(value: string | undefined): ToolDetail | undefined {
  return value === undefined || value === "" ? undefined : { kind: "text", value };
}

function toolDetail(args: Record<string, unknown>): ToolDetail | undefined {
  for (const { key, kind } of TOOL_DETAIL_KEYS) {
    const value = args[key];
    if (value != null) return { kind, value: String(value) };
  }
  return undefined;
}
