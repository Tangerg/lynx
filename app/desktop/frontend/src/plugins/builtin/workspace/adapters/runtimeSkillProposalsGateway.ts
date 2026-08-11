import { getContainer } from "@/main/container";
import { configureSkillProposalsGateway } from "../application/ports/skillProposalsGateway";
import type { SkillProposalsGateway } from "../application/ports/skillProposalsGateway";

const gateway: SkillProposalsGateway = {
  async approve(handle) {
    const { workspace, ...ref } = handle;
    const resources = await getContainer().client().workspaces.open({ path: workspace });
    await resources.skills.approveProposal(ref);
  },
  async reject(handle) {
    const { workspace, ...ref } = handle;
    const resources = await getContainer().client().workspaces.open({ path: workspace });
    await resources.skills.rejectProposal(ref);
  },
};

export function installSkillProposalsGateway(): () => void {
  return configureSkillProposalsGateway(gateway);
}
