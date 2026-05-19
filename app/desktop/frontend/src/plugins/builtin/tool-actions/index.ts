// Built-in plugin: starter set of ToolCard header actions.
//
// "Copy command" is the most universally useful — for `bash` tools it
// lets the user paste the exact command into a terminal. The predicate
// gates it to tools where copying the args is meaningful.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.tool-actions",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerAction({
      id: "copy-args",
      icon: "copy",
      title: "Copy command",
      order: 0,
      predicate: (tool) => tool.args.trim().length > 0,
      run: async (tool) => {
        if (typeof navigator !== "undefined" && navigator.clipboard) {
          try { await navigator.clipboard.writeText(tool.args); } catch {
            /* clipboard write can fail in unfocused windows; ignore */
          }
        }
      },
    });
  },
});
