import { getContainer } from "@/main/container";
import { configureSkillProposalsGateway } from "../application/ports/skillProposalsGateway";
import type { SkillProposalsGateway } from "../application/ports/skillProposalsGateway";

// Bound to the runtime's default workspace — the same one the review queue was read
// from. skills.proposals.* is workspace-scoped, and a decision resolved against a
// different workspace than the list would act on a proposal nobody reviewed.
const resources = () => getContainer().client().workspaces.open();

const gateway: SkillProposalsGateway = {
  async approve(handle) {
    await (await resources()).skills.approveProposal(handle);
  },
  async reject(handle) {
    await (await resources()).skills.rejectProposal(handle);
  },
};

export function installSkillProposalsGateway(): () => void {
  return configureSkillProposalsGateway(gateway);
}
