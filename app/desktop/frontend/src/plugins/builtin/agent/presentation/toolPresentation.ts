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

const TOOL_LABELS: Record<string, string> = {
  _: "Shell",
  shell: "Shell",
  bash: "Shell",
  read: "Read",
  edit: "Edit",
  write: "Write",
  grep: "Grep",
  glob: "Glob",
  lsp: "LSP",
};

const TOOL_DETAIL_KEYS = ["command", "file_path", "path", "query", "pattern"] as const;

const READ_ONLY_TOOLS = new Set(["read", "grep", "glob", "lsp"]);

export function toolIntent(tool: ToolCall): ToolIntent {
  const parsed = parseToolArgs(tool.args);
  return {
    label: TOOL_LABELS[tool.fn] ?? tool.fn,
    detail: parsed ? toolDetail(parsed) : undefined,
  };
}

export function toolMetaItems(tool: ToolCall): ToolMetaItem[] {
  const items: ToolMetaItem[] = [];
  if (tool.added != null) {
    items.push({ id: "added", label: `+${tool.added}`, tone: "success" });
  }
  if (tool.removed != null) {
    items.push({ id: "removed", label: `-${tool.removed}`, tone: "negative" });
  }
  if (tool.hits != null) {
    items.push({ id: "hits", label: `${tool.hits} matches`, tone: "muted" });
  }
  if (tool.exitCode != null && tool.exitCode !== 0) {
    items.push({ id: "exit", label: `exit ${tool.exitCode}`, tone: "negative" });
  }
  if (tool.status === "running") {
    items.push({ id: "live", label: "live", tone: "muted" });
  }
  return items;
}

export function isReadOnlyTool(name: string): boolean {
  return READ_ONLY_TOOLS.has(name) || name.startsWith("lsp_");
}

export function toolGroupNeedsAttention(tools: readonly ToolCall[]): boolean {
  return tools.some((tool) => tool.status === "running" || tool.status === "err");
}

export function summarizeToolGroup(tools: readonly ToolCall[]): string {
  let read = 0;
  let search = 0;
  let lookup = 0;
  for (const tool of tools) {
    if (tool.name === "read") read++;
    else if (tool.name === "lsp" || tool.name.startsWith("lsp_")) lookup++;
    else search++;
  }

  const parts: string[] = [];
  if (read) parts.push(`${read} read`);
  if (search) parts.push(`${search} search`);
  if (lookup) parts.push(`${lookup} lookup`);
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
