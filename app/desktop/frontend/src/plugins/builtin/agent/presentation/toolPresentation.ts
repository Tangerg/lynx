import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ActivityShell } from "@/lib/activityShell";
import { fmtDuration } from "@/lib/format";

export interface ToolIntent {
  label: string;
  detail?: string;
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

const TOOL_DETAIL_KEYS = ["path", "query", "pattern", "url"] as const;

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const parsed = parseToolArgs(tool.args);
  // Only when the projection had nothing better than the tool's own name to show
  // (`fn` is normally the command or the path it acted on). Keyed on that
  // condition rather than on `fn` matching a name in the table, which is the same
  // thing by coincidence until a shell command happens to be spelled `grep`.
  const labelKey = tool.fn === tool.name ? TOOL_LABEL_KEYS[tool.name] : undefined;
  return {
    label: labelKey ? t(labelKey) : tool.fn,
    // A shell call's command comes from its own field; everything else falls back
    // to whatever identifying argument its arg text happens to carry.
    detail: tool.command ?? (parsed ? toolDetail(parsed) : undefined),
  };
}

export function toolMetaItems(t: Translate, tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  if (tool.added != null) {
    items.push({
      id: "added",
      label: t("tool.meta.added", { count: tool.added }),
      tone: "success",
    });
  }
  if (tool.removed != null) {
    items.push({
      id: "removed",
      label: t("tool.meta.removed", { count: tool.removed }),
      tone: "negative",
    });
  }
  if (tool.files != null) {
    items.push({ id: "files", label: t("tool.meta.files", { count: tool.files }), tone: "muted" });
  }
  if (tool.hits != null) {
    items.push({ id: "hits", label: t("tool.meta.matches", { count: tool.hits }), tone: "muted" });
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
  if (tool.durationMs != null && tool.durationMs >= 1000) {
    items.push({ id: "duration", label: fmtDuration(tool.durationMs), tone: "muted" });
  }
  if (tool.status === "running") {
    items.push({ id: "live", label: t("tool.meta.live"), tone: "muted" });
  }
  return items;
}

/**
 * Whether a call had no side effect the user would need to approve.
 *
 * The runtime's own answer, stamped on the toolCall Item — the same table the
 * approval gate consults, so the row's weight and the gate's decision cannot
 * disagree. This used to be a hand-kept list of tool names here, which is a second
 * answer to a question the protocol already answers, and it went stale the moment
 * a tool was renamed.
 */
export function isReadOnlyTool(tool: ToolCall): boolean {
  return tool.safetyClass === "safe";
}

/**
 * How much of the plane one tool call claims.
 *
 * A table and not a chain of conditions in the card, because this is the whole of
 * the taxonomy and it wants to be readable in one place. The read/produce split is
 * the interesting half: a turn can hold a dozen reads and one command, and giving
 * all thirteen the same card is what makes a transcript one grey stack.
 *
 * The state cases come first, because a read that FAILED is no longer a glance —
 * it is the thing you opened the transcript to find.
 *
 * The read/produce split reads the runtime's own safety class, so a tool added or
 * renamed on the backend arrives correctly weighted with no table to update here.
 */
export function toolActivityShell(tool: ToolCall): ActivityShell {
  if (tool.status === "err" || tool.status === "denied" || tool.status === "requires-action") {
    return "flagged";
  }
  return isReadOnlyTool(tool) ? "line" : "card";
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

export function summarizeToolGroup(t: Translate, tools: readonly ToolCall[]): string {
  let read = 0;
  let search = 0;
  let lookup = 0;
  for (const tool of tools) {
    if (tool.name === "read") read++;
    else if (tool.name === "lsp") lookup++;
    else search++;
  }

  const parts: string[] = [];
  if (read) parts.push(t("tool.group.read", { count: read }));
  if (search) parts.push(t("tool.group.search", { count: search }));
  if (lookup) parts.push(t("tool.group.lookup", { count: lookup }));
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

function toolDetail(args: Record<string, unknown>): string | undefined {
  for (const key of TOOL_DETAIL_KEYS) {
    const value = args[key];
    if (value != null) return String(value);
  }
  return undefined;
}
