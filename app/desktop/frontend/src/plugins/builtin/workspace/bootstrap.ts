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
import {
  activateWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { WORKSPACE_SCOPE_PORTS } from "@/plugins/builtin/workspace/public/ports";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  provides: { scopes: WORKSPACE_SCOPE_PORTS },
  setup(ctx) {
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
    ctx.cleanup(() => {
      for (let index = disposers.length - 1; index >= 0; index--) disposers[index]!();
    });
    return {
      scopes: {
        activateSessionScope: activateWorkspaceSessionScope,
        forgetSessionScopes: forgetWorkspaceSessionScopes,
      },
    };
  },
});
