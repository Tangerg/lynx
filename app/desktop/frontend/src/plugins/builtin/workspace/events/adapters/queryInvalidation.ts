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
import { RECIPES_KEY } from "@/plugins/builtin/chat/recipes/public/queries";
import { HOOKS_KEY } from "@/plugins/builtin/settings/hooks/public/queries";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/queries";
import { USAGE_SUMMARY_KEY } from "@/plugins/builtin/settings/usage/public/queries";
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/plugins/builtin/settings/providers/public/queries";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
} from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
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
  diff: WORKSPACE_DIFF_KEY,
  fileHead: WORKSPACE_FILE_HEAD_KEY,
  fileList: WORKSPACE_LIST_FILES_KEY,
  fileRead: WORKSPACE_READ_FILE_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
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
    replaceWorkspaceReadModels();
    return;
  }
  // One scoped resync may name Goal together with Run/HITL/Plan. Start the
  // transactionally coherent mounted-session replacement once, before walking
  // the remaining independent query targets.
  if (targets.includes("agentSessionProjection")) {
    synchronizeMountedAgentSessions({ sessionIds });
  }
  for (const target of targets) {
    if (target === "all") continue;
    if (target === "agentSessionProjection") {
      continue;
    }
    replaceCachedRead({ queryKey: [QUERY_KEYS[target]] });
  }
}

export function invalidateWorkspaceEvent(ev: WorkspaceEventLike): void {
  invalidateWorkspaceTargets(workspaceInvalidations(ev), ev.sessionIds);
}

export function invalidateWorkspaceEverything(): void {
  replaceWorkspaceReadModels();
}

/** Phase one of a Runtime generation handoff. Retire every writer admitted by
 * the prior connection while preserving the last committed projection. The
 * event loop starts phase two only after its successor tail is open. */
export function retireWorkspaceReadModels(): void {
  synchronizeMountedAgentSessions({ ownership: "retire-live" });
  void queryClient.cancelQueries();
}

/** Replace all server-owned material after the configured endpoint commits.
 * Unlike a transient disconnect, facts from another server scope cannot remain
 * as an offline projection while successor reads start. */
export function replaceWorkspaceServerScope(): void {
  synchronizeMountedAgentSessions({ ownership: "replace-server" });
  void queryClient.resetQueries();
}

function replaceWorkspaceReadModels(): void {
  // The material session projection is not a query-cache entry. Replace its
  // live generation first. Its sessions.snapshot successor now carries Goal
  // from the same SQLite transaction, so an independent goals.get writer for a
  // mounted Session would split the generation this boundary is replacing.
  synchronizeMountedAgentSessions({ ownership: "replace-live" });
  replaceCachedRead();
}

export function replaceCachedRead(options?: {
  queryKey: readonly unknown[];
  exact?: boolean;
}): void {
  // A query with no cached value normally reuses its in-flight Promise when it
  // is invalidated. Both a committed change event and a Runtime replacement
  // must retire that writer before starting the successor read; late settlement
  // remains owned by TanStack Query's canceled retryer and cannot populate the
  // cache.
  if (options) {
    void queryClient.cancelQueries(options);
    void queryClient.invalidateQueries(options);
    return;
  }
  void queryClient.cancelQueries();
  void queryClient.invalidateQueries();
}
