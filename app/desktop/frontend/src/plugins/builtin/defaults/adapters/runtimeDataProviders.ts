import type {
  ApprovalRulesQuery,
  DiffQuery,
  FileChangesQuery,
  FileHeadQuery,
  GrepQuery,
  HooksQuery,
  ListFilesQuery,
  McpToolsQuery,
  MemoryQuery,
  ReadFileQuery,
  RecipesQuery,
  WorkspaceDiff,
} from "@/lib/data/queries";
import type { DataProviderSpec, Host } from "@/plugins/sdk";
import type { McpServer as RpcMCPServer } from "@/rpc";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
  CODEBASE_STATUS_KEY,
  DIFF_KEY,
  EMBEDDING_ROLE_KEY,
  FILES_CHANGED_KEY,
  HOOKS_KEY,
  MCP_CONFIGS_KEY,
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  MEMORY_KEY,
  MODELS_KEY,
  PROJECTS_KEY,
  PROVIDERS_KEY,
  RECIPES_KEY,
  SCHEDULES_KEY,
  SESSIONS_KEY,
  SKILLS_KEY,
  UTILITY_ROLE_KEY,
} from "@/lib/data/queries";
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
    key: SESSIONS_KEY,
    fetcher: async () => (await client().sessions.list()).data.map(toAgentSessionSummary),
  });
  contribute({
    key: PROJECTS_KEY,
    fetcher: async () =>
      (await client().workspace.listProjects()).data.map(toWorkspaceProjectSummary),
  });
  contribute({
    key: FILES_CHANGED_KEY,
    fetcher: async (params) =>
      (
        await client().workspace.listFileChanges((params as FileChangesQuery | undefined)?.cwd)
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
    key: DIFF_KEY,
    fetcher: async (params) => {
      const diff = await client().workspace.getDiff({ ...(params as DiffQuery), format: "rows" });
      return { files: diff.files ?? [], truncated: diff.truncated } satisfies WorkspaceDiff;
    },
  });
  contribute({
    key: "grep",
    fetcher: (params) => client().workspace.grep(params as GrepQuery),
  });
  contribute({
    key: "file-head",
    fetcher: async (params) =>
      (await client().workspace.getFileHead(params as FileHeadQuery)).lines,
  });
  contribute({
    key: SKILLS_KEY,
    fetcher: async () =>
      (await client().workspace.listSkills().catch(emptyPageIfUngated)).data.map((s) => ({
        name: s.name,
        description: s.description ?? "",
        source: s.source ?? "",
      })),
  });
  contribute({
    key: "builtin-tools",
    fetcher: async () =>
      (await client().tools.list()).data.map((t) => ({
        name: t.name,
        description: t.description ?? "",
        safetyClass: t.safetyClass,
      })),
  });
  contribute({
    key: MEMORY_KEY,
    fetcher: async (params) =>
      (
        await client()
          .memory.list((params as MemoryQuery | undefined)?.cwd)
          .catch(emptyPageIfUngated)
      ).data.map((m) => ({
        scope: m.scope,
        path: m.path,
        content: m.content,
        updatedAt: m.updatedAt,
      })),
  });
  contribute({
    key: "agent-docs",
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
    key: "list-files",
    fetcher: async (params) => {
      const q = params as ListFilesQuery;
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
    key: "read-file",
    fetcher: async (params) => {
      const q = params as ReadFileQuery;
      const r = await client().workspace.readFile({ path: q.path, cwd: q.cwd });
      return { content: r.content, totalLines: r.totalLines, truncated: r.truncated };
    },
  });
}
