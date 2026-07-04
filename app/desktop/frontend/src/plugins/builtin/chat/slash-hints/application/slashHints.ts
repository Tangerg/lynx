import type { SlashCommandSpec } from "@/plugins/sdk";

export type Translate = (key: string) => string;

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

export function slashHintContributions(t: Translate): SlashHintContribution[] {
  return DEFAULT_SLASH_HINTS.map((hint) => ({
    cmd: hint.cmd,
    spec: { description: t(hint.descriptionKey) },
  }));
}
