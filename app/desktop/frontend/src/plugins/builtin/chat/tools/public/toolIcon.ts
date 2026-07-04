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

import type { IconName } from "@/ui/icons";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";

/** The icon/preview registry key for a tool = its wire `name` (§4.4). */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

// Built-in tool `name` → icon glyph (§4.4.2 display conventions). Every tool
// with bespoke rendering gets a glyph that fits what it DOES — read / write /
// edit must not collapse to one "file", or a glance can't tell them apart.
// Tools with no entry fall through to the generic toolbox glyph (`tool`).
export const DEFAULT_TOOL_ICONS: Record<string, IconName> = {
  // Shell — terminal is the shared domain glyph; the background ops split out
  // by what they do (start a process / read its output / kill it).
  shell: "terminal",
  run_in_background: "play",
  shell_output: "list",
  shell_kill: "stop",
  // File ops — a distinct verb each so read ≠ write ≠ edit at a glance.
  read: "eye", // view contents
  write: "file-plus", // create / overwrite a file
  edit: "edit", // modify in place (pencil)
  // Search — content match vs filename pattern.
  grep: "search",
  glob: "folder-search",
  // Web — query vs retrieve.
  web_search: "globe",
  web_fetch: "download",
  // Code intelligence.
  lsp: "code",
  lsp_diagnostics: "bug", // surfaced problems
  // Agentic.
  skill: "sparkle",
  task: "spark",
  subagent: "bot", // a spawned sub-agent
  ask_user: "question", // asking the human a question, not generic chat
};

export function toolIconFor(key: string): IconName {
  const registered = lookupExtensionByKey(TOOL_ICON, key);
  if (registered) return registered as IconName;
  // Unlisted lsp_* tools still resolve to code; everything else to the generic
  // toolbox glyph — a tool we don't render specially still reads as "a tool".
  return DEFAULT_TOOL_ICONS[key] ?? (key.startsWith("lsp_") ? "code" : "tool");
}
