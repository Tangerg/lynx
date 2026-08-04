import type { ApprovalRulesQuery } from "@/plugins/builtin/agent/public/approvalPolicy";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { RECIPES_KEY, type RecipesQuery } from "@/plugins/builtin/chat/recipes/public/queries";
import {
  GOAL_KEY,
  type GoalQuery,
  type GoalState,
} from "@/plugins/builtin/chat/goal/public/queries";
import { HOOKS_KEY, type HooksQuery } from "@/plugins/builtin/settings/hooks/public/queries";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type McpToolsQuery,
} from "@/plugins/builtin/settings/mcp-servers/public/queries";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  type CodebaseStatusQuery,
} from "@/plugins/builtin/settings/providers/public/queries";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/queries";
import type {
  WorkspaceDiffQuery,
  WorkspaceFileChangesQuery,
  WorkspaceFileHeadQuery,
  WorkspaceGrepQuery,
  WorkspaceListFilesQuery,
  WorkspaceMemoryQuery,
  WorkspaceReadFileQuery,
  WorkspaceDiff,
  AgentMemoryQuery,
} from "@/plugins/builtin/workspace/public/queries";
import {
  WORKSPACE_AGENT_DOCS_KEY,
  WORKSPACE_BUILTIN_TOOLS_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_FILE_HEAD_KEY,
  WORKSPACE_GREP_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_MEMORY_KEY,
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
} from "@/plugins/builtin/workspace/public/queries";
import type { DataProviderSpec, ContributingHost } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asSessionId } from "@/rpc";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  emptyListIfUngated,
  toMCPServerSettings,
  toWorkspaceFileChangeSummary,
  toWorkspaceProjectSummary,
  toAgentSessionSummary,
} from "./runtimeReadModelAdapters";

// DATA_PROVIDER intentionally erases each fetcher's parameter type so unlike
// resources can share one registry. Restore that type once at this adapter
// boundary instead of scattering unchecked casts through every provider.
function optionalParams<P>(params: unknown): P | undefined {
  return params as P | undefined;
}

function requiredParams<P>(key: string, params: unknown): P {
  const value = optionalParams<P>(params);
  if (value === undefined) throw new Error(`Data provider "${key}" requires parameters`);
  return value;
}

function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}

export function registerDefaultDataProviders(host: ContributingHost): void {
  const client = () => getContainer().client();
  const workspace = (cwd?: string) => client().workspaces.open(cwd ? { path: cwd } : undefined);
  const contribute = (provider: DataProviderSpec): void => {
    host.extensions.contribute(DATA_PROVIDER, provider);
  };

  contribute({
    key: AGENT_SESSIONS_KEY,
    fetcher: async () =>
      (await client().sessions.list().autoPagingToArray()).map(toAgentSessionSummary),
  });
  contribute({
    key: WORKSPACE_PROJECTS_KEY,
    fetcher: async () =>
      (await pageData(client().workspaces.list())).map(toWorkspaceProjectSummary),
  });
  contribute({
    key: WORKSPACE_FILES_CHANGED_KEY,
    fetcher: async (params) => {
      const resources = await workspace(optionalParams<WorkspaceFileChangesQuery>(params)?.cwd);
      return (await pageData(resources.changes.list())).map(toWorkspaceFileChangeSummary);
    },
  });
  contribute({
    key: MCP_SERVERS_KEY,
    // One server entry carries both configuration and live state. listTools is
    // reserved for the detail pane's input-schema view.
    fetcher: async () =>
      (await pageData(client().mcp.list()).catch(emptyListIfUngated)).map(toMCPServerSettings),
  });
  contribute({
    key: MCP_TOOLS_KEY,
    fetcher: async (params) =>
      (
        await pageData(
          client().mcp.listTools(requiredParams<McpToolsQuery>(MCP_TOOLS_KEY, params).server),
        ).catch(emptyListIfUngated)
      ).map((t) => ({ name: t.name, description: t.description ?? "" })),
  });
  contribute({
    key: WORKSPACE_DIFF_KEY,
    fetcher: async (params) => {
      const { cwd, ...query } = requiredParams<WorkspaceDiffQuery>(WORKSPACE_DIFF_KEY, params);
      const resources = await workspace(cwd);
      const diff = await resources.diff.get({ ...query, format: "rows" });
      return { files: diff.files ?? [], truncated: diff.truncated } satisfies WorkspaceDiff;
    },
  });
  contribute({
    key: WORKSPACE_GREP_KEY,
    fetcher: async (params) => {
      const { cwd, ...query } = requiredParams<WorkspaceGrepQuery>(WORKSPACE_GREP_KEY, params);
      return (await workspace(cwd)).files.search(query);
    },
  });
  contribute({
    key: WORKSPACE_FILE_HEAD_KEY,
    fetcher: async (params) => {
      const { cwd, ...query } = requiredParams<WorkspaceFileHeadQuery>(
        WORKSPACE_FILE_HEAD_KEY,
        params,
      );
      const resources = await workspace(cwd);
      return (await resources.files.head(query)).lines;
    },
  });
  contribute({
    key: WORKSPACE_SKILLS_KEY,
    fetcher: async () => {
      const resources = await workspace();
      return (await pageData(resources.skills.listDiscovered()).catch(emptyListIfUngated)).map(
        (s) => ({
          name: s.name,
          description: s.description ?? "",
          scope: s.scope,
        }),
      );
    },
  });
  contribute({
    key: WORKSPACE_MANAGED_SKILLS_KEY,
    fetcher: async () =>
      (await pageData(client().skills.listLibrary()).catch(emptyListIfUngated)).map((s) => ({
        name: s.name,
        description: s.description ?? "",
        lifecycle: s.lifecycle,
      })),
  });
  contribute({
    key: WORKSPACE_SKILL_PROPOSALS_KEY,
    fetcher: async () => {
      const resources = await workspace();
      return (await pageData(resources.skills.listProposals()).catch(emptyListIfUngated)).map(
        (p) => ({
          name: p.name,
          revision: p.revision,
          scope: p.scope,
          description: p.description,
          instructions: p.instructions,
          // Absent means the agent decided on its own to distil this.
          origin: p.origin ?? "mined",
          revises: p.revises === true,
          sourceSession: p.sourceSession ?? "",
        }),
      );
    },
  });
  contribute({
    key: WORKSPACE_BUILTIN_TOOLS_KEY,
    fetcher: async () =>
      (await pageData(client().tools.list())).map((t) => ({
        name: t.name,
        description: t.description ?? "",
        safetyClass: t.safetyClass,
      })),
  });
  contribute({
    key: WORKSPACE_MEMORY_KEY,
    fetcher: async (params) => {
      const resources = await workspace(optionalParams<WorkspaceMemoryQuery>(params)?.cwd);
      return (await pageData(resources.memory.list()).catch(emptyListIfUngated)).map((m) => ({
        scope: m.scope,
        path: m.path,
        content: m.content,
        updatedAt: m.updatedAt,
      }));
    },
  });
  contribute({
    key: WORKSPACE_AGENT_MEMORY_KEY,
    fetcher: async (params) => {
      const q = requiredParams<AgentMemoryQuery>(WORKSPACE_AGENT_MEMORY_KEY, params);
      if (!runtimeCapability("agentMemory")) return [];
      const result =
        q.scope === "user"
          ? await client().agentMemory.list({ scope: "user" })
          : await workspace(q.cwd).then((resources) => resources.agentMemory.list());
      return result.items.map((m) => ({
        id: m.id,
        scope: m.scope,
        content: m.content,
        origin: m.origin,
        status: m.status,
        pinned: m.pinned,
        sessionId: m.sessionId ?? "",
        day: m.day ?? "",
        createdAt: m.createdAt,
        updatedAt: m.updatedAt,
      }));
    },
  });
  contribute({
    key: GOAL_KEY,
    fetcher: async (params) => {
      const { sessionId } = requiredParams<GoalQuery>(GOAL_KEY, params);
      if (!runtimeCapability("goals")) {
        return { available: false, goal: null } satisfies GoalState;
      }
      const goal = await client().goals.get(asSessionId(sessionId));
      return {
        available: true,
        goal: goal
          ? {
              sessionId: goal.sessionId,
              objective: goal.objective,
              status: goal.status,
              stop: goal.reason
                ? { code: goal.reason.code, detail: goal.reason.detail ?? "" }
                : null,
              budget: {
                maxRuns: goal.budget.maxRuns ?? 0,
                maxCostUsd: goal.budget.maxCostUsd ?? 0,
                maxSteps: goal.budget.maxSteps ?? 0,
              },
              used: {
                runs: goal.used.runs,
                costUsd: goal.used.costUsd,
                steps: goal.used.steps,
              },
            }
          : null,
      } satisfies GoalState;
    },
  });
  contribute({
    key: WORKSPACE_AGENT_DOCS_KEY,
    fetcher: async () => {
      const resources = await workspace();
      return (await pageData(resources.agentDocs.list()).catch(emptyListIfUngated)).map((d) => ({
        path: d.path,
        title: d.title ?? "",
        scope: d.scope,
      }));
    },
  });
  contribute({
    key: MODELS_KEY,
    // Aggregate models across enabled providers only; catalog-only providers
    // cannot run and would produce dead composer-picker options.
    fetcher: async () => {
      const enabled = (await pageData(client().providers.list())).filter(
        (p) => p.apiKeyMasked !== "",
      );
      const lists = await Promise.all(
        enabled.map((p) => pageData(client().models.list(p.id)).catch(() => [])),
      );
      return lists.flat().map((m) => ({
        id: m.id,
        provider: m.provider,
        label: m.displayName ?? m.id,
        multimodal: m.capabilities?.multimodal ?? false,
        contextWindow: m.contextWindow,
      }));
    },
  });
  contribute({
    key: PROVIDERS_KEY,
    fetcher: async () =>
      (await pageData(client().providers.list())).map((p) => ({
        id: p.id,
        baseUrl: p.baseUrl ?? "",
        apiKeyMasked: p.apiKeyMasked,
        keySource: p.keySource,
        embeddingCapable: p.embeddingCapable,
        defaultEmbeddingModel: p.defaultEmbeddingModel,
      })),
  });
  contribute({
    key: APPROVAL_MODE_KEY,
    fetcher: async () => (await client().approval.getMode()).mode,
  });
  contribute({
    key: UTILITY_ROLE_KEY,
    fetcher: () => client().models.getUtilityRole(),
  });
  contribute({
    key: EMBEDDING_ROLE_KEY,
    fetcher: () => client().models.getEmbeddingRole(),
  });
  contribute({
    key: CODEBASE_STATUS_KEY,
    fetcher: (params) => {
      if (!runtimeCapability("codebase")) {
        return Promise.resolve({ state: "none" as const, fileCount: 0, chunkCount: 0 });
      }
      return workspace(optionalParams<CodebaseStatusQuery>(params)?.cwd).then((resources) =>
        resources.codebase.status(),
      );
    },
  });
  contribute({
    key: APPROVAL_RULES_KEY,
    fetcher: async (params) => {
      const query = requiredParams<ApprovalRulesQuery>(APPROVAL_RULES_KEY, params);
      return (await client().approval.listRules(asSessionId(query.sessionId))).rules;
    },
  });
  contribute({
    key: HOOKS_KEY,
    fetcher: async (params) =>
      (await workspace(optionalParams<HooksQuery>(params)?.cwd)).hooks.list(),
  });
  contribute({
    key: SCHEDULES_KEY,
    fetcher: async () => {
      if (!runtimeCapability("schedules")) return [];
      return client().schedules.list().autoPagingToArray();
    },
  });
  contribute({
    key: RECIPES_KEY,
    fetcher: async (params) => {
      const resources = await workspace(optionalParams<RecipesQuery>(params)?.cwd);
      return pageData(resources.recipes.list()).catch(emptyListIfUngated);
    },
  });
  contribute({
    key: WORKSPACE_LIST_FILES_KEY,
    fetcher: async (params) => {
      const { cwd, ...query } = requiredParams<WorkspaceListFilesQuery>(
        WORKSPACE_LIST_FILES_KEY,
        params,
      );
      const resources = await workspace(cwd);
      return (await resources.files.list(query).autoPagingToArray()).map((e) => ({
        path: e.path,
        name: e.name,
        type: e.type,
        sizeBytes: e.sizeBytes,
      }));
    },
  });
  contribute({
    key: WORKSPACE_READ_FILE_KEY,
    fetcher: async (params) => {
      const { cwd, ...query } = requiredParams<WorkspaceReadFileQuery>(
        WORKSPACE_READ_FILE_KEY,
        params,
      );
      const r = await (await workspace(cwd)).files.read(query);
      return { content: r.content, totalLines: r.totalLines, truncated: r.truncated };
    },
  });
}
