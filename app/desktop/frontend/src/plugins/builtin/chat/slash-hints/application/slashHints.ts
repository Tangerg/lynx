import type { SlashCommandSpec } from "@/plugins/sdk";

export interface SlashHintDefinition {
  cmd: string;
  descriptionKey: string;
}

export interface SlashHintContribution {
  cmd: string;
  spec: SlashCommandSpec;
}

export const DEFAULT_SLASH_HINTS: SlashHintDefinition[] = [
  { cmd: "/explain", descriptionKey: "slash.explain" },
  { cmd: "/test", descriptionKey: "slash.test" },
  { cmd: "/fix", descriptionKey: "slash.fix" },
  { cmd: "/diff", descriptionKey: "slash.diff" },
  { cmd: "/review", descriptionKey: "slash.review" },
  { cmd: "/commit", descriptionKey: "slash.commit" },
  { cmd: "/search", descriptionKey: "slash.search" },
  { cmd: "/plan", descriptionKey: "slash.plan" },
];

/**
 * The hints as specs, carrying their KEYS.
 *
 * The suggestion list resolves the description itself (`t(spec.description)`), so
 * handing it a finished sentence pinned every hint to whichever language happened
 * to be loaded when this plugin registered — a switch afterwards moved the rest of
 * the UI and left these eight behind. It went unnoticed because translating an
 * already-translated string returns it unchanged, so nothing ever looked broken in
 * the language it started in.
 */
export function slashHintContributions(): SlashHintContribution[] {
  return DEFAULT_SLASH_HINTS.map((hint) => ({
    cmd: hint.cmd,
    spec: { description: hint.descriptionKey },
  }));
}
