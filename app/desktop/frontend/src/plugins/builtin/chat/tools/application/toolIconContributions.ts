export interface ToolIconContribution {
  key: string;
  icon: string;
}

// Built-in tool name -> icon glyph. The same table feeds registry contributions
// and the no-plugin fallback, so built-in rendering cannot drift.
export const DEFAULT_TOOL_ICONS: Record<string, string> = {
  shell: "terminal",
  run_in_background: "play",
  shell_output: "list",
  shell_kill: "stop",
  read: "eye",
  write: "file-plus",
  edit: "edit",
  grep: "search",
  glob: "folder-search",
  web_search: "globe",
  web_fetch: "download",
  lsp: "code",
  lsp_diagnostics: "bug",
  skill: "sparkle",
  task: "spark",
  subagent: "bot",
  ask_user: "question",
};

export function defaultToolIconContributions(): ToolIconContribution[] {
  return Object.entries(DEFAULT_TOOL_ICONS).map(([key, icon]) => ({ key, icon }));
}

export function defaultToolIconFor(key: string): string {
  return DEFAULT_TOOL_ICONS[key] ?? (key.startsWith("lsp_") ? "code" : "tool");
}
