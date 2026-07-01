import { queryClient } from "@/lib/data/queryClient";
import {
  DIFF_KEY,
  FILES_CHANGED_KEY,
  MCP_CONFIGS_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  SESSIONS_KEY,
  SKILLS_KEY,
} from "@/lib/data/queries";
import {
  workspaceInvalidations,
  type WorkspaceEventLike,
  type WorkspaceInvalidationTarget,
} from "../domain/eventInvalidation";

const QUERY_KEYS: Record<Exclude<WorkspaceInvalidationTarget, "all">, string> = {
  diff: DIFF_KEY,
  filesChanged: FILES_CHANGED_KEY,
  mcpConfigs: MCP_CONFIGS_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  sessions: SESSIONS_KEY,
  skills: SKILLS_KEY,
};

export function invalidateWorkspaceTargets(targets: WorkspaceInvalidationTarget[]): void {
  if (targets.includes("all")) {
    void queryClient.invalidateQueries();
    return;
  }
  for (const target of targets) {
    if (target === "all") continue;
    void queryClient.invalidateQueries({ queryKey: [QUERY_KEYS[target]] });
  }
}

export function invalidateWorkspaceEvent(ev: WorkspaceEventLike): void {
  invalidateWorkspaceTargets(workspaceInvalidations(ev));
}

export function invalidateWorkspaceEverything(): void {
  void queryClient.invalidateQueries();
}
