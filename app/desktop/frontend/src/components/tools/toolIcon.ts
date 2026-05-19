// Resolve a tool fn name to the icon glyph rendered inside ToolCard.
//
// Lookup order:
//   1. Plugin registry — `host.tool.registerIcon("bash", "terminal")`. Lets
//      a plugin add an icon for a custom tool, or swap the built-in one.
//   2. Hardcoded fallback — kept inline so the shell still renders sensibly
//      even with zero plugins loaded (and so the built-in mapping plugin
//      is a thin convenience, not the source of truth).
//
// The fallback list mirrors `lyra.builtin.tool-icons` exactly — keeping
// the two in sync is the trade-off for being able to render before the
// plugin loads (initial paint).

import type { IconName } from "@/components/common/Icon";
import { lookupToolIcon } from "@/plugins/sdk";

export function toolIconFor(fn: string): IconName {
  const registered = lookupToolIcon(fn);
  if (registered) return registered as IconName;

  if (fn === "read_file" || fn === "write_file" || fn === "edit_file") return "file";
  if (fn === "grep") return "search";
  if (fn === "bash") return "terminal";
  if (fn === "web_search") return "globe";
  return "tool";
}
