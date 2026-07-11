import type { ApprovalRulesQuery } from "@/plugins/builtin/agent/public/approvalPolicy";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { RECIPES_KEY, type RecipesQuery } from "@/plugins/builtin/chat/recipes/public/data";
import { HOOKS_KEY, type HooksQuery } from "@/plugins/builtin/settings/hooks/public/data";
import {
  MCP_CONFIGS_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  type McpToolsQuery,
} from "@/plugins/builtin/settings/mcp-servers/public/data";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/plugins/builtin/settings/providers/public/data";
import { SCHEDULES_KEY } from "@/plugins/builtin/settings/schedules/public/data";
import type {
  WorkspaceDiffQuery,
  WorkspaceFileChangesQuery,
  WorkspaceFileHeadQuery,
  WorkspaceGrepQuery,
  WorkspaceListFilesQuery,
  WorkspaceMemoryQuery,
  WorkspaceReadFileQuery,
  WorkspaceDiff,
} from "@/plugins/builtin/workspace/public/data";
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
} from "@/plugins/builtin/workspace/public/data";
import type { DataProviderSpec, Host } from "@/plugins/sdk";
import type { McpServer as RpcMCPServer } from "@/rpc";
import { getContainer } from "@/main/container";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asSessionId } from "@/rpc";
import {
  emptyPageIfUngated,
  toMcpConfigInfo,
  toWorkspaceFileChangeSummary,
  toMcpServerStatusSummary,
  toWorkspaceProjectSummary,
  toAgentSessionSummary,
} from "./runtimeDataAdapters";

export function registerDefaultDataProviders(host: Host): void {
  const client = () => getContainer().client();
  const contribute = (provider: DataProviderSpec): void => {
    host.extensions.contribute(DATA_PROVIDER, provider);
  };

  contribute({
    key: AGENT_SESSIONS_KEY,
    fetcher: async () => (await client().sessions.list()).data.map(toAgentSessionSummary),
  });
  contribute({
    key: WORKSPACE_PROJECTS_KEY,
    fetcher: async () =>
      (await client().workspace.listProjects()).data.map(toWorkspaceProjectSummary),
  });
  contribute({
    key: WORKSPACE_FILES_CHANGED_KEY,
    fetcher: async (params) =>
      (
        await client().workspace.listFileChanges(
          (params as WorkspaceFileChangesQuery | undefined)?.cwd,
        )
      ).data.map(toWorkspaceFileChangeSummary),
  });
  contribute({
    key: MCP_SERVERS_KEY,
    // listServers entries already carry status/toolCount/error; listTools is
    // reserved for the detail pane's paginated inputSchema view.
    fetcher: async () =>
      (await client().workspace.mcp.listServers().catch(emptyPageIfUngated)).data.map(
        toMcpServerStatusSummary,
      ),
  });
  contribute({
    key: MCP_CONFIGS_KEY,
    fetcher: async () => {
      const [cfgs, srvs] = await Promise.all([
        client().workspace.mcp.listConfigs().catch(emptyPageIfUngated),
        client().workspace.mcp.listServers().catch(emptyPageIfUngated),
      ]);
      const live = new Map<string, RpcMCPServer>(srvs.data.map((s) => [s.name, s]));
      return cfgs.data.map((c) => toMcpConfigInfo(c, live.get(c.name)));
    },
  });
  contribute({
    key: MCP_TOOLS_KEY,
    fetcher: async (params) =>
      (
        await client()
          .workspace.mcp.listTools((params as McpToolsQuery).server)
          .catch(emptyPageIfUngated)
      ).data.map((t) => ({ name: t.name, description: t.description ?? "" })),
  });
  contribute({
    key: WORKSPACE_DIFF_KEY,
    fetcher: async (params) => {
      const diff = await client().workspace.getDiff({
        ...(params as WorkspaceDiffQuery),
        format: "rows",
      });
      return { files: diff.files ?? [], truncated: diff.truncated } satisfies WorkspaceDiff;
    },
  });
  contribute({
    key: WORKSPACE_GREP_KEY,
    fetcher: (params) => client().workspace.grep(params as WorkspaceGrepQuery),
  });
  contribute({
    key: WORKSPACE_FILE_HEAD_KEY,
    fetcher: async (params) =>
      (await client().workspace.getFileHead(params as WorkspaceFileHeadQuery)).lines,
  });
  contribute({
    key: WORKSPACE_SKILLS_KEY,
    fetcher: async () =>
      (await client().workspace.listSkills().catch(emptyPageIfUngated)).data.map((s) => ({
        name: s.name,
        description: s.description ?? "",
        source: s.source ?? "",
      })),
  });
  contribute({
    key: WORKSPACE_BUILTIN_TOOLS_KEY,
    fetcher: async () =>
      (await client().tools.list()).data.map((t) => ({
        name: t.name,
        description: t.description ?? "",
        safetyClass: t.safetyClass,
      })),
  });
  contribute({
    key: WORKSPACE_MEMORY_KEY,
    fetcher: async (params) =>
      (
        await client()
          .memory.list((params as WorkspaceMemoryQuery | undefined)?.cwd)
          .catch(emptyPageIfUngated)
      ).data.map((m) => ({
        scope: m.scope,
        path: m.path,
        content: m.content,
        updatedAt: m.updatedAt,
      })),
  });
  contribute({
    key: WORKSPACE_AGENT_DOCS_KEY,
    fetcher: async () =>
      (await client().workspace.listAgentDocs().catch(emptyPageIfUngated)).data.map((d) => ({
        path: d.path,
        title: d.title ?? "",
        scope: d.scope,
      })),
  });
  contribute({
    key: MODELS_KEY,
    // Aggregate models across enabled providers only; catalog-only providers
    // cannot run and would produce dead composer-picker options.
    fetcher: async () => {
      const enabled = (await client().providers.list()).data.filter((p) => p.apiKeyMasked !== "");
      const lists = await Promise.all(
        enabled.map((p) =>
          client()
            .models.list(p.id)
            .then((r) => r.data)
            .catch(() => []),
        ),
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
      (await client().providers.list()).data.map((p) => ({
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
    fetcher: (params) => client().codebase.status((params as { cwd?: string } | undefined)?.cwd),
  });
  contribute({
    key: APPROVAL_RULES_KEY,
    fetcher: async (params) =>
      (await client().approval.listRules(asSessionId((params as ApprovalRulesQuery).sessionId)))
        .rules,
  });
  contribute({
    key: HOOKS_KEY,
    fetcher: (params) => client().workspace.hooks.list((params as HooksQuery | undefined)?.cwd),
  });
  contribute({
    key: SCHEDULES_KEY,
    fetcher: async () => (await client().schedules.list()).schedules,
  });
  contribute({
    key: RECIPES_KEY,
    fetcher: async (params) =>
      (
        await client()
          .workspace.recipes.list((params as RecipesQuery | undefined)?.cwd)
          .catch(emptyPageIfUngated)
      ).data,
  });
  contribute({
    key: WORKSPACE_LIST_FILES_KEY,
    fetcher: async (params) => {
      const q = params as WorkspaceListFilesQuery;
      return (
        await client().workspace.listFiles({
          cwd: q.cwd,
          path: q.path,
          recursive: q.recursive,
          limit: q.limit,
        })
      ).data.map((e) => ({
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
      const q = params as WorkspaceReadFileQuery;
      const r = await client().workspace.readFile({ path: q.path, cwd: q.cwd });
      return { content: r.content, totalLines: r.totalLines, truncated: r.truncated };
    },
  });
}
