// Resolve a tool to its icon glyph + the registry key used for icon/preview
// lookup.
//
// Routing key (the SAME key drives TOOL_ICON and TOOL_PREVIEW):
//   - typed variants (commandExecution / fileChange / search / webSearch) →
//     the `kind` IS the identity (their `fn` is a display label — a command
//     string / query / path — not a stable name).
//   - the generic `tool` envelope → the tool `name` (= `fn`).
//
// Icon lookup order:
//   1. Plugin registry — `host.extensions.contribute(TOOL_ICON, "terminal", { key: "commandExecution" })`.
//   2. Hardcoded fallback — kept inline so the kernel renders sensibly with
//      zero plugins loaded (the built-in mapping plugin is a thin convenience,
//      not the source of truth). Mirrors `lyra.builtin.tool-icons` exactly.

import type { IconName } from "@/components/common/Icon";
import type { ToolCall } from "@/protocol/run/viewState";
import { lookupExtensionByKey, TOOL_ICON } from "@/plugins/sdk";

/** The icon/preview registry key for a tool: kind for typed variants, the
 *  tool name (`fn`) for the generic `tool` envelope. */
export function toolRoutingKey(tool: ToolCall): string {
  return tool.kind === "tool" ? tool.fn : tool.kind;
}

export function toolIconFor(key: string): IconName {
  const registered = lookupExtensionByKey(TOOL_ICON, key);
  if (registered) return registered as IconName;

  // Typed variants — keyed by kind.
  if (key === "commandExecution") return "terminal";
  if (key === "fileChange") return "file";
  if (key === "search") return "search";
  if (key === "webSearch") return "globe";
  // Generic-tool names.
  if (key === "read" || key === "read_file" || key === "write_file" || key === "edit_file")
    return "file";
  if (key === "grep") return "search";
  if (key === "bash") return "terminal";
  if (key === "web_search") return "globe";
  return "tool";
}
