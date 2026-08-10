/**
 * The built-in tools, in families, each with the one glyph it wears.
 *
 * Two facts about a tool NAME and nothing else, which is why this sits in lib and
 * not in either context that reads it: the transcript needs the glyph (chat/tools
 * contributes this table to the icon registry) and the catalog needs the family
 * (the workspace Tools view groups by it), and those two contexts may not import
 * each other. Same placement, and the same reason, as `activityShell`.
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
 * Colour is deliberately NOT part of the differentiation: tone means state
 * (running, failed, refused), and spending it on tool identity would leave a
 * failed read and a successful one looking equally alarming. The shapes carry the
 * variety; the palette keeps its job.
 *
 * It is a vocabulary for tool NAMES, not an inventory of one runtime's catalog: a
 * name this table covers but the connected runtime does not expose costs nothing,
 * and a name it does expose that is missing here still renders (generic glyph,
 * unplaced in the catalog). Which tools exist is `tools.list`'s answer, and every
 * consumer here takes it from there.
 *
 * The FAMILIES are what someone browsing the catalog is asking about — can it run
 * commands, does it remember, can it reach the network — and deliberately not the
 * runtime's safety classes: 22 of the 30 are `safe`, so that taxonomy sorts the
 * catalog into one long bucket and three short ones. The safety class stays on the
 * row, where it answers the other question.
 */
export interface ToolFamily {
  /** i18n key suffix — `tools.family.<id>`. */
  id: string;
  tools: readonly { name: string; icon: string }[];
}

export const TOOL_FAMILIES: readonly ToolFamily[] = [
  {
    // A prompt, its scrollback, and the stop.
    id: "shell",
    tools: [
      { name: "shell", icon: "terminal" },
      { name: "read_shell_output", icon: "scroll" },
      { name: "stop_shell", icon: "stop" },
    ],
  },
  {
    // Read, create, amend, and apply someone else's diff.
    id: "files",
    tools: [
      { name: "read", icon: "eye" },
      { name: "write", icon: "file-plus" },
      { name: "edit", icon: "edit" },
      { name: "apply_patch", icon: "replace" },
    ],
  },
  {
    // Finding things — inside files, by filename, and by symbol.
    id: "search",
    tools: [
      { name: "grep", icon: "text-search" },
      { name: "glob", icon: "folder-search" },
      { name: "lsp", icon: "code" },
    ],
  },
  {
    // Three different acts: query the web, pull one page, call an endpoint.
    id: "network",
    tools: [
      { name: "web_search", icon: "globe" },
      { name: "web_fetch", icon: "download" },
      { name: "http_request", icon: "webhook" },
    ],
  },
  {
    // The shelf, one taken down, a page out of it, and a new one offered.
    id: "skills",
    tools: [
      { name: "list_skills", icon: "library" },
      { name: "load_skill", icon: "book-open" },
      { name: "read_skill_resource", icon: "paperclip" },
      { name: "propose_skill", icon: "sparkle" },
    ],
  },
  {
    // Asking: another agent, or the person.
    id: "delegation",
    tools: [
      { name: "delegate_task", icon: "users" },
      { name: "ask_user", icon: "question" },
    ],
  },
  {
    // Plan mode — entering it, writing the plan, leaving it.
    id: "plan",
    tools: [
      { name: "enter_plan_mode", icon: "map" },
      { name: "set_plan", icon: "list-checks" },
      { name: "exit_plan_mode", icon: "flag" },
    ],
  },
  {
    // What it remembered, what was said before, what tools exist, and a result it
    // had set aside.
    id: "recall",
    tools: [
      { name: "search_memory", icon: "brain" },
      { name: "search_conversations", icon: "history" },
      { name: "search_tools", icon: "package-search" },
      { name: "read_tool_result", icon: "archive" },
    ],
  },
  {
    // Read the calendar, add to it, take something off it.
    id: "schedules",
    tools: [
      { name: "list_schedules", icon: "clock" },
      { name: "create_schedule", icon: "calendar-plus" },
      { name: "delete_schedule", icon: "calendar-x" },
    ],
  },
  {
    // Set the target, check the target, report what came of it.
    id: "goals",
    tools: [
      { name: "create_goal", icon: "target" },
      { name: "get_goal", icon: "crosshair" },
      { name: "report_goal_outcome", icon: "clipboard-check" },
    ],
  },
];

/**
 * Which family a tool name belongs to, or `undefined` for one this table has never
 * heard of — an MCP tool, or a built-in added on the backend before the client
 * learns its glyph. Nothing here fabricates a family for it; the catalog gives the
 * unplaced ones their own heading at the end.
 */
export function toolFamilyId(name: string): string | undefined {
  return FAMILY_BY_TOOL.get(name);
}

const FAMILY_BY_TOOL = new Map<string, string>(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, family.id] as const)),
);

export const TOOL_ICON_BY_NAME: Readonly<Record<string, string>> = Object.fromEntries(
  TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => [tool.name, tool.icon])),
);
