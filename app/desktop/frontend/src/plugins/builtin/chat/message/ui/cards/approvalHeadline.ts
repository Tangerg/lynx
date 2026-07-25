import { toolCategory } from "@/plugins/builtin/agent/public/viewState";

// "What am I being asked to allow?" — phrased per tool family, in the language
// the user is reading. Derived at render rather than at fold time: the fold ran
// once, so a headline baked there kept the wording it was born with even after
// the user switched languages.
export function approvalHeadline(
  t: (key: string, params?: Record<string, string | number>) => string,
  toolName: string | undefined,
): string {
  if (!toolName) return t("approval.fallbackText");
  switch (toolCategory(toolName)) {
    case "command":
      return t("approval.what.command");
    case "fileEdit":
      return t("approval.what.fileEdit");
    case "search":
      return t("approval.what.search");
    case "webSearch":
      return t("approval.what.webSearch");
    case "subagent":
      return t("approval.what.subagent");
    default:
      return t("approval.what.generic", { tool: toolName });
  }
}
