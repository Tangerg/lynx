import { defaultToolIconContributions } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { copyText } from "@/lib/clipboard";
import { definePlugin } from "@/plugins/sdk";
import { TOOL_ACTION, TOOL_ICON } from "@/plugins/sdk/kernelPoints";
import { copyToolArgsAction } from "./application/toolActions";

export const toolActions = definePlugin({
  name: "scopeapp.builtin.tool-actions",
  setup(ctx) {
    ctx.contribute(TOOL_ACTION, copyToolArgsAction({ title: "toolAction.copyCommand", copyText }));
  },
});

export const toolIcons = definePlugin({
  name: "scopeapp.builtin.tool-icons",
  setup(ctx) {
    for (const { key, icon } of defaultToolIconContributions()) {
      ctx.contribute(TOOL_ICON, icon, { key });
    }
  },
});
