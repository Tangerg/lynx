export interface ToolIconContribution {
  key: string;
  icon: string;
}

// Built-in tool name -> icon glyph. The same table feeds registry contributions
// and the no-plugin fallback, so built-in rendering cannot drift.
export const DEFAULT_TOOL_ICONS: Record<string, string> = {
  shell: "terminal",
  read_shell_output: "list",
  stop_shell: "stop",
  read: "eye",
  write: "file-plus",
  edit: "edit",
  apply_patch: "edit",
  grep: "search",
  glob: "folder-search",
  web_search: "globe",
  web_fetch: "download",
  http_request: "globe",
  lsp: "code",
  list_skills: "sparkle",
  load_skill: "sparkle",
  read_skill_resource: "sparkle",
  propose_skill: "sparkle",
  delegate_task: "spark",
  ask_user: "question",
  enter_plan_mode: "list",
  set_plan: "list",
  exit_plan_mode: "list",
  search_memory: "search",
  search_conversations: "search",
  search_tools: "search",
  read_tool_result: "list",
  list_schedules: "clock",
  create_schedule: "clock",
  delete_schedule: "clock",
  create_goal: "loop",
  get_goal: "loop",
  report_goal_outcome: "loop",
};

export function defaultToolIconContributions(): ToolIconContribution[] {
  return Object.entries(DEFAULT_TOOL_ICONS).map(([key, icon]) => ({ key, icon }));
}

export function defaultToolIconFor(key: string): string {
  return DEFAULT_TOOL_ICONS[key] ?? "tool";
}
