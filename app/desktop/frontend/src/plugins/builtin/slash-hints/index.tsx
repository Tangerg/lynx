// Built-in slash hints. These are *display-only* — typing one of them shows
// the description in the autocomplete dropdown, but pressing Enter just
// sends the text as a normal user message. Concrete commands with `run`
// handlers come from plugins.

import { definePlugin } from "@/plugins/sdk";

const HINTS: Array<[cmd: string, description: string]> = [
  ["/explain", "Explain a file, function, or selection"],
  ["/test",    "Generate or run tests for the current change"],
  ["/fix",     "Diagnose and fix the failing typecheck"],
  ["/diff",    "Show the working-tree diff inline"],
  ["/review",  "Review pending changes line-by-line"],
  ["/commit",  "Stage, commit, and push the current branch"],
  ["/search",  "Search the codebase for a symbol or pattern"],
  ["/plan",    "Restate or edit the current plan"],
];

export default definePlugin({
  name: "lyra.builtin.slash-hints",
  version: "1.0.0",
  setup({ host }) {
    for (const [cmd, description] of HINTS) {
      host.composer.registerCommand(cmd, { description });
    }
  },
});
