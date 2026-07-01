import { definePlugin } from "@/plugins/sdk";
import { installCodebaseGateway } from "./adapters/runtimeCodebaseGateway";
import { installWorkspaceMemoryGateway } from "./adapters/runtimeMemoryGateway";
import { installToolCatalogGateway } from "./adapters/runtimeToolCatalogGateway";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  version: "1.0.0",
  requires: ["lyra.builtin.bootstrap"],
  setup() {
    installCodebaseGateway();
    installWorkspaceMemoryGateway();
    installToolCatalogGateway();
    installWorkspaceNavigationPort();
  },
});
