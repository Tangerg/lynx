// Which family a runtime tool belongs to — the map from tool NAME to the way the
// UI talks about it (a command, a file edit, a search, a delegation).
//
// This is the agent context's vocabulary: `shell`, `edit`, `grep`, `web_search`,
// `task` are the runtime's tool names, and what they mean is this context's
// business. It lived in the kernel's plugin-contract types, where nothing in the
// kernel used it and this context re-exported it through its own facade — the
// rule in one place and its owner merely forwarding.

export type ToolCategory =
  | "command" // shell → { command } + { output, exitCode? }, or a plain-string ack when backgrounded
  | "fileEdit" // edit / write → { file_path } + { changes: FileEdit[] }
  | "search" // grep / glob → { pattern } + { hits: SearchHit[] }
  | "webSearch" // web_search → { query } + { results: WebSearchResult[] }
  | "read" // read → { file_path, offset?, limit? } + { content, start_line, … }
  | "subagent" // task → { description, prompt } + a plain-string reply
  | "generic"; // MCP "<server>_<tool>" / anything unknown → JSON tree

const TOOL_CATEGORY: Record<string, ToolCategory> = {
  shell: "command",
  edit: "fileEdit",
  write: "fileEdit",
  grep: "search",
  glob: "search",
  web_search: "webSearch",
  read: "read",
  task: "subagent", // the runtime's delegation tool (spawns a child run, returns its reply)
};
// lsp / lsp_diagnostics / skill / ask_user / shell_output / shell_kill stay
// "generic" on purpose: their labels, icons, and previews key on the tool NAME
// (projections.toolLabel + TOOL_ICON + TOOL_PREVIEW), and their results are
// plain text the generic field projection already passes through.

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
