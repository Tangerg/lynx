import { DEFAULT_TOOL_ICONS } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { copyText } from "@/lib/clipboard";
import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_ACTION, TOOL_ICON } from "@/plugins/sdk/kernelPoints";
import { copyToolArgsAction } from "./application/toolActions";

export const toolActions = definePlugin({
  name: "lyra.builtin.tool-actions",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(
      TOOL_ACTION,
      copyToolArgsAction({ title: t("toolAction.copyCommand"), copyText }),
    );
  },
});

export const toolIcons = definePlugin({
  name: "lyra.builtin.tool-icons",
  version: "1.0.0",
  setup({ host }) {
    for (const [key, icon] of Object.entries(DEFAULT_TOOL_ICONS)) {
      host.extensions.contribute(TOOL_ICON, icon, { key });
    }
  },
});
