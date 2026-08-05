export interface ToolIconContribution {
  key: string;
  icon: string;
}

/**
 * Built-in tool name → icon glyph. The same table feeds registry contributions
 * and the no-plugin fallback, so built-in rendering cannot drift.
 *
 * ONE GLYPH PER TOOL, and a test holds it that way. This ran 16 glyphs across 32
 * tools: `list` stood for reading shell output, three plan-mode calls and a
 * deferred result; `search` for grep, two recall families and the tool catalog;
 * `sparkle` for all four Skill calls; `clock` for all three schedules; `loop` for
 * all three goal calls. A scrolled transcript was a column of four repeating
 * shapes, which is the same as no shape at all — the glyph is the only part of a
 * folded row a reader takes in without reading, and it was spending that on
 * nothing.
 *
 * Colour is deliberately NOT part of the differentiation: tone here means state
 * (running, failed, refused), and spending it on tool identity would leave a
 * failed read and a successful one looking equally alarming. Thirty-two shapes
 * carry the variety; the palette keeps its job.
 */
export const DEFAULT_TOOL_ICONS: Record<string, string> = {
  // Shell — a prompt, its scrollback, and the stop.
  shell: "terminal",
  read_shell_output: "scroll",
  stop_shell: "stop",

  // Files — read, create, amend, and apply someone else's diff.
  read: "eye",
  write: "file-plus",
  edit: "edit",
  apply_patch: "replace",

  // Finding things — inside files, by filename, and by symbol.
  grep: "text-search",
  glob: "folder-search",
  lsp: "code",

  // The network, whose three calls are three different acts: query the web,
  // pull one page, call an endpoint.
  web_search: "globe",
  web_fetch: "download",
  http_request: "webhook",

  // Skills — the shelf, one taken down, a page out of it, and a new one offered.
  list_skills: "library",
  load_skill: "book-open",
  read_skill_resource: "paperclip",
  propose_skill: "sparkle",

  // Asking: another agent, or the person.
  delegate_task: "users",
  ask_user: "question",

  // Plan mode — entering it, writing the plan, leaving it.
  enter_plan_mode: "map",
  set_plan: "list-checks",
  exit_plan_mode: "flag",

  // Recall — what it remembered, what was said before, what tools exist, and a
  // result it had set aside.
  search_memory: "brain",
  search_conversations: "history",
  search_tools: "package-search",
  read_tool_result: "archive",

  // Schedules — read the calendar, add to it, take something off it.
  list_schedules: "clock",
  create_schedule: "calendar-plus",
  delete_schedule: "calendar-x",

  // A Goal: set the target, check the target, report what came of it.
  create_goal: "target",
  get_goal: "crosshair",
  report_goal_outcome: "clipboard-check",
};

export function defaultToolIconContributions(): ToolIconContribution[] {
  return Object.entries(DEFAULT_TOOL_ICONS).map(([key, icon]) => ({ key, icon }));
}

export function defaultToolIconFor(key: string): string {
  return DEFAULT_TOOL_ICONS[key] ?? "tool";
}
