import { getContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { SkillCurationOwner } from "../application/skillCuration";
import type { SkillCurationGateway } from "../application/ports/skillCurationGateway";

function runtimeSkillCurationGateway(client: LyraClient): SkillCurationGateway {
  return {
    archive: (name) => client.skills.archive(name),
    restore: (name) => client.skills.restore(name),
    async approveProposal(handle) {
      const { workspace, ...ref } = handle;
      const resources = await client.workspaces.open({ path: workspace });
      await resources.skills.approveProposal(ref);
    },
    async rejectProposal(handle) {
      const { workspace, ...ref } = handle;
      const resources = await client.workspaces.open({ path: workspace });
      await resources.skills.rejectProposal(ref);
    },
  };
}

export function installSkillCurationGateway() {
  const gateway = runtimeSkillCurationGateway(getContainer().client());
  const owner = SkillCurationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
