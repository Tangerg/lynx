import { definePlugin, TOOL_VIEW_OPENER } from "@/plugins/sdk";
import { workspaceToolViewOpener } from "../application/toolViewOpenerContributions";

export default definePlugin({
  name: "lyra.builtin.workspace.tool-view-opener",
  setup(ctx) {
    ctx.contribute(TOOL_VIEW_OPENER, workspaceToolViewOpener());
  },
});
