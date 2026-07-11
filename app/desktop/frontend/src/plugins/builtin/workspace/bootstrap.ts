import { definePlugin } from "@/plugins/sdk";
import { installCodebaseGateway } from "./adapters/runtimeCodebaseGateway";
import { installConversationArchiveGateway } from "./adapters/runtimeConversationArchiveGateway";
import { installWorkspaceMemoryGateway } from "./adapters/runtimeMemoryGateway";
import { installToolCatalogGateway } from "./adapters/runtimeToolCatalogGateway";
import { installWorkspaceErrorClassifier } from "./adapters/runtimeWorkspaceErrorClassifier";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  version: "1.0.0",
  setup() {
    const disposers = [
      installCodebaseGateway(),
      installConversationArchiveGateway(),
      installWorkspaceMemoryGateway(),
      installToolCatalogGateway(),
      installWorkspaceErrorClassifier(),
      installWorkspaceNavigationPort(),
    ];
    return () => {
      for (let index = disposers.length - 1; index >= 0; index--) disposers[index]!();
    };
  },
});
