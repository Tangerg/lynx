import { queryClient } from "@/lib/data/queryClient";
import { skillLibraryGateway } from "./ports/skillLibraryGateway";
import { WORKSPACE_MANAGED_SKILLS_KEY, WORKSPACE_SKILLS_KEY } from "./workspaceData";

// Archiving or restoring changes both the management view and the agent's
// discovery view (an archived skill is no longer loadable), so refresh both.
async function invalidate(): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_MANAGED_SKILLS_KEY] }),
    queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILLS_KEY] }),
  ]);
}

export async function archiveSkill(name: string): Promise<void> {
  await skillLibraryGateway().archive(name);
  await invalidate();
}

export async function restoreSkill(name: string): Promise<void> {
  await skillLibraryGateway().restore(name);
  await invalidate();
}
