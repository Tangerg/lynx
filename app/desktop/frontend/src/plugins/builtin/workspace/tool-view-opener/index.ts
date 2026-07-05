import { definePlugin, TOOL_VIEW_OPENER } from "@/plugins/sdk";
import { workspaceToolViewOpener } from "../application/toolViewOpenerContributions";

export default definePlugin({
  name: "lyra.builtin.workspace.tool-view-opener",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(TOOL_VIEW_OPENER, workspaceToolViewOpener());
  },
});
