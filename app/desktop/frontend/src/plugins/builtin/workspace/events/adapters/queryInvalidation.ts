import { queryClient } from "@/lib/data/queryClient";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
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
} from "@/plugins/builtin/workspace/public/data";
import {
  workspaceInvalidations,
  type WorkspaceEventLike,
  type WorkspaceInvalidationTarget,
} from "../domain/eventInvalidation";

const QUERY_KEYS: Record<Exclude<WorkspaceInvalidationTarget, "all">, string> = {
  diff: WORKSPACE_DIFF_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
  mcpConfigs: MCP_CONFIGS_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  sessions: AGENT_SESSIONS_KEY,
  skills: WORKSPACE_SKILLS_KEY,
  managedSkills: WORKSPACE_MANAGED_SKILLS_KEY,
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
