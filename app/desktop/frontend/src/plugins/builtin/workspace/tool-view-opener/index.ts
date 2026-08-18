import { definePlugin, TOOL_VIEW_OPENER } from "@/plugins/sdk";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "../application/toolRouting";

export default definePlugin({
  name: "lyra.builtin.workspace.tool-view-opener",
  setup(ctx) {
    ctx.contribute(TOOL_VIEW_OPENER, {
      id: "workspace-tool-view",
      order: 0,
      predicate: hasWorkspaceViewForTool,
      open: openWorkspaceViewForTool,
    });
  },
});
