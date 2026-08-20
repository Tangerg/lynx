// Which family a runtime tool belongs to — the map from tool NAME to the way the
// UI talks about it (a command, a file edit, a search, a delegation).
//
// This is the agent context's vocabulary: `shell`, `edit`, `grep`, `web_search`,
// `delegate_task` are the runtime's tool names, and what they mean is this
// context's business. It lived in the kernel's plugin-contract types, where
// nothing in the kernel used it and this context re-exported it through its own
// facade — the rule in one place and its owner merely forwarding.

export type ToolCategory =
  | "command" // shell → { command, description } + { output, exitCode? }, or a plain-string ack when backgrounded
  | "fileEdit" // apply_patch → { patch } + { changes: AppliedChange[] }
  | "search" // grep / glob → { pattern } + { hits: SearchHit[] }
  | "webSearch" // web_search → { query } + { results: WebSearchResult[] }
  | "read" // read → { path, start_line?, max_lines? } + { content, start_line, … }
  | "subagent" // delegate_task → { summary, instructions } + a plain-string reply
  | "generic"; // MCP "<server>_<tool>" / anything unknown → JSON tree

const TOOL_CATEGORY: Record<string, ToolCategory> = {
  shell: "command",
  // The Runtime's only built-in file mutation. Its result is a call-scoped
  // receipt, not a workspace diff and not an edit/write compatibility shape.
  apply_patch: "fileEdit",
  grep: "search",
  glob: "search",
  web_search: "webSearch",
  read: "read",
  delegate_task: "subagent", // the runtime's delegation tool (spawns a child run, returns its reply)
};
// Everything else stays "generic" on purpose: their labels, icons, and previews key
// on the tool NAME (projections.nameLabel + TOOL_ICON + TOOL_PREVIEW), and their
// results are plain text the generic field projection already passes through.

export function toolCategory(name: string): ToolCategory {
  return TOOL_CATEGORY[name] ?? "generic";
}

// HITL question tools: ask_user / exit_plan_mode call hitl.Interrupt from inside
// their own Call, so the runtime emits BOTH a toolCall Item (started, then
// drained to `incomplete` when the turn parks — §5.2) AND a question Item. The
// QuestionCard is the real representation; the tool row is its redundant shadow
// (and reads as a red ✗ via the incomplete→err mapping), so the renderer drops
// it whenever the question block is present.
const QUESTION_TOOLS = new Set(["ask_user", "exit_plan_mode"]);
export function isQuestionTool(name: string): boolean {
  return QUESTION_TOOLS.has(name);
}
