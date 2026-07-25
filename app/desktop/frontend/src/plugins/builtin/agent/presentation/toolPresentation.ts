import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentView";

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
  grep: "tool.label.grep",
  glob: "tool.label.glob",
  lsp: "tool.label.lsp",
};

const TOOL_DETAIL_KEYS = ["command", "file_path", "path", "query", "pattern"] as const;

const READ_ONLY_TOOLS = new Set(["read", "grep", "glob", "lsp"]);

export function toolIntent(t: Translate, tool: ToolCall): ToolIntent {
  const parsed = parseToolArgs(tool.args);
  const labelKey = TOOL_LABEL_KEYS[tool.fn];
  return {
    label: labelKey ? t(labelKey) : tool.fn,
    detail: parsed ? toolDetail(parsed) : undefined,
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
  if (tool.hits != null) {
    items.push({ id: "hits", label: t("tool.meta.matches", { count: tool.hits }), tone: "muted" });
  }
  if (tool.exitCode != null && tool.exitCode !== 0) {
    items.push({
      id: "exit",
      label: t("tool.meta.exit", { code: tool.exitCode }),
      tone: "negative",
    });
  }
  if (tool.status === "running") {
    items.push({ id: "live", label: t("tool.meta.live"), tone: "muted" });
  }
  return items;
}

export function isReadOnlyTool(name: string): boolean {
  return READ_ONLY_TOOLS.has(name) || name.startsWith("lsp_");
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
    else if (tool.name === "lsp" || tool.name.startsWith("lsp_")) lookup++;
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
