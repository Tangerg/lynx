// Built-in plugins: tool-meta — header actions and the fn → icon glyph map.
//
// `tool-icons` mirrors the hardcoded fallback in `toolIconFor` (so first
// paint before plugins load still picks a sensible glyph); this plugin
// is the source of truth that third-party tools extend.

import { definePlugin } from "@/plugins/sdk";

export const toolActions = definePlugin({
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

export const toolIcons = definePlugin({
  name: "lyra.builtin.tool-icons",
  version: "1.0.0",
  setup({ host }) {
    host.tool.registerIcon("read_file",  "file");
    host.tool.registerIcon("write_file", "file");
    host.tool.registerIcon("edit_file",  "file");
    host.tool.registerIcon("grep",       "search");
    host.tool.registerIcon("bash",       "terminal");
    host.tool.registerIcon("web_search", "globe");
  },
});
