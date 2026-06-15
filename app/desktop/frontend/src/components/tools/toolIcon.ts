// Resolve a tool to its icon glyph + the registry key used for icon/preview
// lookup.
//
// The protocol core is domain-neutral: every tool is `{ name, arguments,
// result }` (API.md §4.4), so the routing key is simply the tool `name` — the
// SAME key drives TOOL_ICON and TOOL_PREVIEW.
//
// `DEFAULT_TOOL_ICONS` is the single source of truth for the built-in name →
// glyph mapping: the `tool-icons` plugin contributes every entry to the
// TOOL_ICON point (so third-party tools extend the same surface), and the
// `toolIconFor` fallback indexes it directly so the kernel still renders
// sensibly with zero plugins loaded. One table, two readers — they can't drift.

import type { IconName } from "@/components/common/Icon";
import type { ToolCall } from "@/protocol/run/viewState";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";

/** The icon/preview registry key for a tool = its wire `name` (§4.4). */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

/** Built-in tool `name` → icon glyph (§4.4.2 display conventions). */
export const DEFAULT_TOOL_ICONS: Record<string, IconName> = {
  bash: "terminal",
  shell: "terminal",
  run_in_background: "terminal",
  bash_output: "terminal",
  kill_shell: "stop",
  edit: "file",
  write: "file",
  read: "file",
  grep: "search",
  glob: "search",
  web_search: "globe",
  web_fetch: "globe",
  lsp: "code",
  lsp_diagnostics: "code",
  skill: "sparkle",
  task: "spark",
  subagent: "spark",
  ask_user: "chat",
};

export function toolIconFor(key: string): IconName {
  const registered = lookupExtensionByKey(TOOL_ICON, key);
  if (registered) return registered as IconName;
  // Unlisted lsp_* tools still resolve to code; everything else to the generic tool glyph.
  return DEFAULT_TOOL_ICONS[key] ?? (key.startsWith("lsp_") ? "code" : "tool");
}
