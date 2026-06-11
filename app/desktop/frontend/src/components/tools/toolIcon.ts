// Resolve a tool to its icon glyph + the registry key used for icon/preview
// lookup.
//
// The protocol core is domain-neutral: every tool is `{ name, arguments,
// result }` (API.md §4.4), so the routing key is simply the tool `name` — the
// SAME key drives TOOL_ICON and TOOL_PREVIEW.
//
// Icon lookup order:
//   1. Plugin registry — `host.extensions.contribute(TOOL_ICON, "terminal", { key: "bash" })`.
//   2. Hardcoded fallback — kept inline so the kernel renders sensibly with
//      zero plugins loaded (the built-in mapping plugin is a thin convenience,
//      not the source of truth). Mirrors `lyra.builtin.tool-icons` exactly.

import type { IconName } from "@/components/common/Icon";
import type { ToolCall } from "@/protocol/run/viewState";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";

/** The icon/preview registry key for a tool = its wire `name` (§4.4). */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.name;
}

export function toolIconFor(key: string): IconName {
  const registered = lookupExtensionByKey(TOOL_ICON, key);
  if (registered) return registered as IconName;

  // Fallback by tool name (§4.4.2 display conventions).
  if (key === "bash" || key === "shell") return "terminal";
  if (key === "edit" || key === "write" || key === "read") return "file";
  if (key === "grep" || key === "glob") return "search";
  if (key === "webSearch") return "globe";
  if (key.startsWith("lsp_")) return "code";
  if (key === "skill") return "sparkle";
  if (key === "task" || key === "subagent") return "spark";
  if (key === "ask_user") return "chat";
  return "tool";
}
