import { queryClient } from "@/lib/data/queryClient";
import { skillDraftsGateway, type SkillDraftHandle } from "./ports/skillDraftsGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_DRAFTS_KEY,
} from "./workspaceData";

// Promoting publishes a draft into the active library, so it changes the drafts
// queue AND both skill views (the library gains a skill, the agent's discovery
// view can load it). Rejecting only drops it from the queue. skills.changed
// fans out to other clients; this local invalidation refreshes the acting one
// without waiting for the event round-trip.
async function invalidatePromote(): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILL_DRAFTS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_MANAGED_SKILLS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILLS_KEY] }),
  ]);
}

export async function promoteSkillDraft(handle: SkillDraftHandle): Promise<void> {
  await skillDraftsGateway().promote(handle);
  await invalidatePromote();
}

export async function rejectSkillDraft(handle: SkillDraftHandle): Promise<void> {
  await skillDraftsGateway().reject(handle);
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILL_DRAFTS_KEY] });
}
