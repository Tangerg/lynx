import { definePlugin } from "@/plugins/sdk";
import { installCodebaseGateway } from "./adapters/runtimeCodebaseGateway";
import { installConversationArchiveGateway } from "./adapters/runtimeConversationArchiveGateway";
import { installWorkspaceKnowledgeGateway } from "./adapters/runtimeKnowledgeGateway";
import { installAgentMemoryGateway } from "./adapters/runtimeAgentMemoryGateway";
import { installSkillLibraryGateway } from "./adapters/runtimeSkillLibraryGateway";
import { installSkillProposalsGateway } from "./adapters/runtimeSkillProposalsGateway";
import { installToolCatalogGateway } from "./adapters/runtimeToolCatalogGateway";
import { installWorkspaceErrorClassifier } from "./adapters/runtimeWorkspaceErrorClassifier";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";
import { installBrowserFileTransfer } from "./adapters/browserFileTransfer";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  version: "1.0.0",
  setup() {
    const disposers = [
      installCodebaseGateway(),
      installConversationArchiveGateway(),
      installWorkspaceKnowledgeGateway(),
      installAgentMemoryGateway(),
      installSkillLibraryGateway(),
      installSkillProposalsGateway(),
      installToolCatalogGateway(),
      installWorkspaceErrorClassifier(),
      installWorkspaceNavigationPort(),
      installBrowserFileTransfer(),
    ];
    return () => {
      for (let index = disposers.length - 1; index >= 0; index--) disposers[index]!();
    };
  },
});
