import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  AGENT_SESSION_USAGE_KEY,
  synchronizeMountedAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import { GOAL_KEY } from "@/plugins/builtin/chat/goal/public/queries";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/queries";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
} from "@/plugins/builtin/settings/mcp-servers/public/queries";
import {
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
} from "@/plugins/builtin/workspace/public/queries";
import {
  workspaceInvalidations,
  type WorkspaceEventLike,
  type WorkspaceInvalidationTarget,
} from "../domain/eventInvalidation";

const QUERY_KEYS: Record<
  Exclude<WorkspaceInvalidationTarget, "all" | "agentSessionProjection">,
  string
> = {
  diff: WORKSPACE_DIFF_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
  goal: GOAL_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  schedules: SCHEDULES_KEY,
  sessions: AGENT_SESSIONS_KEY,
  sessionUsage: AGENT_SESSION_USAGE_KEY,
  skills: WORKSPACE_SKILLS_KEY,
  managedSkills: WORKSPACE_MANAGED_SKILLS_KEY,
  skillProposals: WORKSPACE_SKILL_PROPOSALS_KEY,
};

export function invalidateWorkspaceTargets(
  targets: WorkspaceInvalidationTarget[],
  sessionIds?: readonly string[],
): void {
  if (targets.includes("all")) {
    void queryClient.invalidateQueries();
    // The material session projection is not a query-cache entry. A global
    // resync therefore invokes its authoritative synchronization explicitly.
    synchronizeMountedAgentSessions();
    return;
  }
  for (const target of targets) {
    if (target === "all") continue;
    if (target === "agentSessionProjection") {
      synchronizeMountedAgentSessions(sessionIds);
      continue;
    }
    void queryClient.invalidateQueries({ queryKey: [QUERY_KEYS[target]] });
  }
}

export function invalidateWorkspaceEvent(ev: WorkspaceEventLike): void {
  invalidateWorkspaceTargets(workspaceInvalidations(ev), ev.sessionIds);
}

export function invalidateWorkspaceEverything(): void {
  void queryClient.invalidateQueries();
  synchronizeMountedAgentSessions();
}
