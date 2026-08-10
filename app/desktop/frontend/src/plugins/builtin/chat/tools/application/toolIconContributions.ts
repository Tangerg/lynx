import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";

export interface ToolIconContribution {
  key: string;
  icon: string;
}

/**
 * Built-in tool name → icon glyph, from the one table that holds a tool name's
 * presentation facts (see lib/toolFamilies for why the glyphs live there and what
 * rule governs them). The same source feeds registry contributions and the
 * no-plugin fallback, so built-in rendering cannot drift.
 */
export function defaultToolIconContributions(): ToolIconContribution[] {
  return Object.entries(TOOL_ICON_BY_NAME).map(([key, icon]) => ({ key, icon }));
}

export function defaultToolIconFor(key: string): string {
  return TOOL_ICON_BY_NAME[key] ?? "tool";
}
