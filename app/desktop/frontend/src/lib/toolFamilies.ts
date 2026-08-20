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
 * It is the built-in vocabulary, not the live inventory: `tools.list` remains the
 * authority for which tools are currently exposed, while this table assigns a
 * family and glyph to each of the Runtime's 30 built-ins. Unknown MCP tools still
 * render with the generic glyph and remain unplaced in the built-in catalog.
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
    // Read a file or apply one model-authored patch.
    id: "files",
    tools: [
      { name: "read", icon: "eye" },
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
