import { definePlugin } from "@/plugins/sdk";
import { installConversationArchiveGateway } from "./adapters/runtimeConversationArchiveGateway";
import { installWorkspaceKnowledgeGateway } from "./adapters/runtimeKnowledgeGateway";
import { installAgentMemoryGateway } from "./adapters/runtimeAgentMemoryGateway";
import { installSkillCurationGateway } from "./adapters/runtimeSkillCurationGateway";
import { installDiagnosticToolGateway } from "./adapters/runtimeDiagnosticToolGateway";
import { installWorkspaceErrorClassifier } from "./adapters/runtimeWorkspaceErrorClassifier";
import { installWorkspaceNavigationPort } from "./adapters/navigationStatePort";
import {
  activateWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { WORKSPACE_SCOPE_PORTS } from "@/plugins/builtin/workspace/public/ports";
import { WORKSPACE_MUTATION_LIFECYCLE_PORTS } from "@/plugins/builtin/workspace/public/ports";

export default definePlugin({
  name: "lyra.builtin.workspace-bootstrap",
  provides: {
    scopes: WORKSPACE_SCOPE_PORTS,
    mutationLifecycle: WORKSPACE_MUTATION_LIFECYCLE_PORTS,
  },
  setup(ctx) {
    const agentMemory = installAgentMemoryGateway();
    const knowledge = installWorkspaceKnowledgeGateway();
    const skillCuration = installSkillCurationGateway();
    const diagnosticTool = installDiagnosticToolGateway();
    const conversationArchive = installConversationArchiveGateway();
    const disposers = [
      () => conversationArchive.dispose(),
      () => knowledge.dispose(),
      () => agentMemory.dispose(),
      () => skillCuration.dispose(),
      () => diagnosticTool.dispose(),
      installWorkspaceErrorClassifier(),
      installWorkspaceNavigationPort(),
    ];
    ctx.cleanup(() => {
      for (let index = disposers.length - 1; index >= 0; index--) disposers[index]!();
    });
    return {
      scopes: {
        activateSessionScope: activateWorkspaceSessionScope,
        forgetSessionScopes: forgetWorkspaceSessionScopes,
      },
      mutationLifecycle: {
        replaceRuntimeGeneration() {
          knowledge.replaceRuntimeGeneration();
          skillCuration.replaceRuntimeGeneration();
          agentMemory.replaceRuntimeGeneration();
          diagnosticTool.replaceRuntimeGeneration();
          conversationArchive.replaceRuntimeGeneration();
        },
      },
    };
  },
});
