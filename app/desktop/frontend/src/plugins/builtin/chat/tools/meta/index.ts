// Built-in plugins: tool-meta — header actions and the name → icon glyph map.
//
// `tool-icons` contributes every entry of DEFAULT_TOOL_ICONS (the shared source
// of truth in toolIcon.ts) to the TOOL_ICON point, so third-party tools extend
// the same surface; first paint before this plugin loads falls back to the same
// table directly (see toolIconFor).

import { DEFAULT_TOOL_ICONS } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { copyText } from "@/lib/clipboard";
import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_ACTION, TOOL_ICON } from "@/plugins/sdk/kernelPoints";

export const toolActions = definePlugin({
  name: "lyra.builtin.tool-actions",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_ACTION, {
      id: "copy-args",
      icon: "copy",
      title: t("toolAction.copyCommand"),
      order: 0,
      predicate: (tool) => tool.args.trim().length > 0,
      run: async (tool) => {
        await copyText(tool.args);
      },
    });
  },
});

export const toolIcons = definePlugin({
  name: "lyra.builtin.tool-icons",
  version: "1.0.0",
  setup({ host }) {
    // Keyed by tool `name` (the routing key, §4.4.2). DEFAULT_TOOL_ICONS is the
    // shared source of truth, so contributions + fallback can't drift.
    for (const [key, icon] of Object.entries(DEFAULT_TOOL_ICONS)) {
      host.extensions.contribute(TOOL_ICON, icon, { key });
    }
  },
});
