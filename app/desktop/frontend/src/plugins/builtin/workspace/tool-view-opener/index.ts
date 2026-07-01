import { definePlugin, TOOL_VIEW_OPENER } from "@/plugins/sdk";
import { hasWorkspaceToolView } from "../application/toolRouteDecision";
import { openWorkspaceViewForTool } from "../application/toolRouting";

export default definePlugin({
  name: "lyra.builtin.workspace.tool-view-opener",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_VIEW_OPENER, {
      id: "workspace-tool-view",
      order: 0,
      predicate: hasWorkspaceToolView,
      open: openWorkspaceViewForTool,
    });
  },
});
