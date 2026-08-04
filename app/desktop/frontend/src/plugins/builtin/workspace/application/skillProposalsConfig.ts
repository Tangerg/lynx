import { queryClient } from "@/lib/queryClient";
import { skillProposalsGateway, type SkillProposalHandle } from "./ports/skillProposalsGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
} from "./workspaceQueries";

// Approving publishes a proposal into the active library, so it changes the review
// queue AND both skill views (the library gains a skill, the agent's discovery
// view can load it). Rejecting only drops it from the queue. skills.changed
// fans out to other clients; this local invalidation refreshes the acting one
// without waiting for the event round-trip.
async function invalidateApprove(): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_MANAGED_SKILLS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILLS_KEY] }),
  ]);
}

export async function approveSkillProposal(handle: SkillProposalHandle): Promise<void> {
  await skillProposalsGateway().approve(handle);
  await invalidateApprove();
}

export async function rejectSkillProposal(handle: SkillProposalHandle): Promise<void> {
  await skillProposalsGateway().reject(handle);
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] });
}
