import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  AGENT_SESSION_USAGE_KEY,
  synchronizeMountedAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import { PENDING_WORK_KEY } from "@/plugins/builtin/agent/public/hitl";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { GOAL_KEY } from "@/plugins/builtin/chat/goal/public/queries";
import { RECIPES_KEY } from "@/plugins/builtin/chat/recipes/public/queries";
import { HOOKS_KEY } from "@/plugins/builtin/settings/hooks/public/queries";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/queries";
import { USAGE_SUMMARY_KEY } from "@/plugins/builtin/settings/usage/public/queries";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/plugins/builtin/settings/providers/public/queries";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
} from "@/plugins/builtin/settings/mcp-servers/public/queries";
import {
  WORKSPACE_DIFF_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_FILE_HEAD_KEY,
  WORKSPACE_GREP_KEY,
  WORKSPACE_AGENT_DOCS_KEY,
  WORKSPACE_KNOWLEDGE_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_READ_FILE_KEY,
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
  agentDocs: WORKSPACE_AGENT_DOCS_KEY,
  agentMemory: WORKSPACE_AGENT_MEMORY_KEY,
  approvalMode: APPROVAL_MODE_KEY,
  approvalRules: APPROVAL_RULES_KEY,
  codebaseStatus: CODEBASE_STATUS_KEY,
  diff: WORKSPACE_DIFF_KEY,
  fileHead: WORKSPACE_FILE_HEAD_KEY,
  fileList: WORKSPACE_LIST_FILES_KEY,
  fileRead: WORKSPACE_READ_FILE_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
  goal: GOAL_KEY,
  grep: WORKSPACE_GREP_KEY,
  hooks: HOOKS_KEY,
  knowledge: WORKSPACE_KNOWLEDGE_KEY,
  models: MODELS_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  pendingWork: PENDING_WORK_KEY,
  providers: PROVIDERS_KEY,
  recipes: RECIPES_KEY,
  schedules: SCHEDULES_KEY,
  sessions: AGENT_SESSIONS_KEY,
  sessionUsage: AGENT_SESSION_USAGE_KEY,
  usageSummary: USAGE_SUMMARY_KEY,
  utilityRole: UTILITY_ROLE_KEY,
  embeddingRole: EMBEDDING_ROLE_KEY,
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
    if (target === "goal" && sessionIds?.length) {
      for (const sessionId of new Set(sessionIds)) {
        void queryClient.invalidateQueries({
          queryKey: [GOAL_KEY, { sessionId }],
          exact: true,
        });
      }
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
