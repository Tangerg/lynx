// Built-in plugins: tool-meta — header actions and the fn → icon glyph map.
//
// `tool-icons` mirrors the hardcoded fallback in `toolIconFor` (so first
// paint before plugins load still picks a sensible glyph); this plugin
// is the source of truth that third-party tools extend.

import { definePlugin } from "@/plugins/sdk";
import { TOOL_ACTION, TOOL_ICON } from "@/plugins/sdk/kernelPoints";

export const toolActions = definePlugin({
  name: "lyra.builtin.tool-actions",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_ACTION, {
      id: "copy-args",
      icon: "copy",
      title: "Copy command",
      order: 0,
      predicate: (tool) => tool.args.trim().length > 0,
      run: async (tool) => {
        if (typeof navigator !== "undefined" && navigator.clipboard) {
          try {
            await navigator.clipboard.writeText(tool.args);
          } catch {
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
    // Typed variants — keyed by ToolInvocation.kind (the routing key).
    host.extensions.contribute(TOOL_ICON, "terminal", { key: "commandExecution" });
    host.extensions.contribute(TOOL_ICON, "file", { key: "fileChange" });
    host.extensions.contribute(TOOL_ICON, "search", { key: "search" });
    host.extensions.contribute(TOOL_ICON, "globe", { key: "webSearch" });
    // Generic-tool names.
    host.extensions.contribute(TOOL_ICON, "file", { key: "read" });
    host.extensions.contribute(TOOL_ICON, "file", { key: "read_file" });
  },
});
