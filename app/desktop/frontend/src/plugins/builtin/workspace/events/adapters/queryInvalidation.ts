import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  AGENT_SESSION_USAGE_KEY,
  getActiveSessionId,
  recoverSessionState,
} from "@/plugins/builtin/agent/public/session";
import { GOAL_KEY } from "@/plugins/builtin/chat/goal/public/data";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/data";
import {
  MCP_CONFIGS_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
} from "@/plugins/builtin/settings/mcp-servers/public/data";
import {
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_DRAFTS_KEY,
} from "@/plugins/builtin/workspace/public/data";
import {
  workspaceInvalidations,
  type WorkspaceEventLike,
  type WorkspaceInvalidationTarget,
} from "../domain/eventInvalidation";

const QUERY_KEYS: Record<Exclude<WorkspaceInvalidationTarget, "all" | "sessionState">, string> = {
  diff: WORKSPACE_DIFF_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
  goal: GOAL_KEY,
  mcpConfigs: MCP_CONFIGS_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  schedules: SCHEDULES_KEY,
  sessions: AGENT_SESSIONS_KEY,
  sessionUsage: AGENT_SESSION_USAGE_KEY,
  skills: WORKSPACE_SKILLS_KEY,
  managedSkills: WORKSPACE_MANAGED_SKILLS_KEY,
  skillDrafts: WORKSPACE_SKILL_DRAFTS_KEY,
};

export function invalidateWorkspaceTargets(targets: WorkspaceInvalidationTarget[]): void {
  if (targets.includes("all")) {
    void queryClient.invalidateQueries();
    // Session-scoped state is not a query: "read everything again" has to include it
    // explicitly or the one read that lives in the agent fold is the one thing a
    // resync does not repair.
    recoverMountedSessionState();
    return;
  }
  for (const target of targets) {
    if (target === "all") continue;
    if (target === "sessionState") {
      recoverMountedSessionState();
      continue;
    }
    void queryClient.invalidateQueries({ queryKey: [QUERY_KEYS[target]] });
  }
}

// The state key's value lands in the agent fold rather than in a query cache, so its
// invalidation is a re-read through the key's recovery method. Only a mounted session
// has a fold to land in; an unmounted one reads it when it opens.
function recoverMountedSessionState(): void {
  const sessionId = getActiveSessionId();
  if (sessionId) void recoverSessionState(sessionId);
}

export function invalidateWorkspaceEvent(ev: WorkspaceEventLike): void {
  invalidateWorkspaceTargets(workspaceInvalidations(ev));
}

export function invalidateWorkspaceEverything(): void {
  void queryClient.invalidateQueries();
  recoverMountedSessionState();
}
