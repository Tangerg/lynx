import { definePlugin } from "@/plugins/sdk";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  version: "1.0.0",
  requires: ["lyra.builtin.bootstrap"],
  setup() {
    installWorkspaceNavigationPort();
  },
});
