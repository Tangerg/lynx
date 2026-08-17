import type { ApprovalRulesQuery } from "@/plugins/builtin/agent/public/approvalPolicy";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { RECIPES_KEY, type RecipesQuery } from "@/plugins/builtin/chat/recipes/public/queries";
import { HOOKS_KEY, type HooksQuery } from "@/plugins/builtin/settings/hooks/public/queries";
import {
  CODEBASE_STATUS_KEY,
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  type CodebaseStatusQuery,
} from "@/plugins/builtin/settings/providers/public/queries";
import {
  SCHEDULES_KEY,
  type ScheduleConfig,
} from "@/plugins/builtin/settings/schedules/public/queries";
import type {
  WorkspaceDiffQuery,
  WorkspaceFileChangesQuery,
  WorkspaceFileHeadQuery,
  WorkspaceGrepQuery,
  WorkspaceListFilesQuery,
  WorkspaceKnowledgeQuery,
  WorkspaceReadFileQuery,
  WorkspaceDiff,
  AgentMemoryQuery,
  WorkspaceCatalogQuery,
} from "@/plugins/builtin/workspace/public/queries";
import {
  WORKSPACE_AGENT_DOCS_KEY,
  WORKSPACE_BUILTIN_TOOLS_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_FILE_HEAD_KEY,
  WORKSPACE_GREP_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_KNOWLEDGE_KEY,
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
} from "@/plugins/builtin/workspace/public/queries";
import type { DataProviderSpec, Contributor } from "@/plugins/sdk";
import { getContainer } from "@/main/container";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { asSessionId, type LyraClient, type Schedule } from "@/rpc";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  emptyListIfUngated,
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

function scheduleConfig(schedule: Schedule): ScheduleConfig {
  const { workspace, ...config } = schedule;
  return {
    ...config,
    ...(workspace ? { cwd: workspace.path } : {}),
  };
}

/** One admitted DATA_PROVIDER read. The Runtime client and cancellation
 * identity are captured together at the query boundary so a multi-stage read
 * cannot splice a retired response into a successor transport. */
class RuntimeProviderRead {
  private constructor(
    readonly client: LyraClient,
    readonly signal: AbortSignal | undefined,
  ) {}

  static begin(signal: AbortSignal | undefined): RuntimeProviderRead {
    return new RuntimeProviderRead(getContainer().client(), signal);
  }

  workspace(cwd?: string) {
    return this.client.workspaces.open(cwd ? { path: cwd } : undefined);
  }
}

interface RuntimeProviderSpec {
  readonly key: string;
  readonly fetcher: (read: RuntimeProviderRead, params?: unknown) => Promise<unknown>;
}

export function registerDefaultDataProviders(ctx: Contributor): void {
  const contribute = ({ key, fetcher }: RuntimeProviderSpec): void => {
    const provider: DataProviderSpec = {
      key,
      fetcher: (params, signal) => fetcher(RuntimeProviderRead.begin(signal), params),
    };
    ctx.contribute(DATA_PROVIDER, provider);
  };

  contribute({
    key: AGENT_SESSIONS_KEY,
    fetcher: async (read) =>
      (await read.client.sessions.list(undefined, read.signal).autoPagingToArray()).map(
        toAgentSessionSummary,
      ),
  });
  contribute({
    key: WORKSPACE_PROJECTS_KEY,
    fetcher: async (read) =>
      (await pageData(read.client.workspaces.list(read.signal))).map(toWorkspaceProjectSummary),
  });
  contribute({
    key: WORKSPACE_FILES_CHANGED_KEY,
    fetcher: async (read, params) => {
      const resources = await read.workspace(
        optionalParams<WorkspaceFileChangesQuery>(params)?.cwd,
      );
      return (await pageData(resources.changes.list(read.signal))).map(
        toWorkspaceFileChangeSummary,
      );
    },
  });
  contribute({
    key: WORKSPACE_DIFF_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceDiffQuery>(WORKSPACE_DIFF_KEY, params);
      const resources = await read.workspace(cwd);
      const diff = await resources.diff.get({ ...query, format: "rows" });
      return { files: diff.files ?? [], truncated: diff.truncated } satisfies WorkspaceDiff;
    },
  });
  contribute({
    key: WORKSPACE_GREP_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceGrepQuery>(WORKSPACE_GREP_KEY, params);
      return (await read.workspace(cwd)).files.search(query);
    },
  });
  contribute({
    key: WORKSPACE_FILE_HEAD_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceFileHeadQuery>(
        WORKSPACE_FILE_HEAD_KEY,
        params,
      );
      const resources = await read.workspace(cwd);
      return (await resources.files.head(query)).lines;
    },
  });
  contribute({
    key: WORKSPACE_SKILLS_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILLS_KEY, params);
      const resources = await read.workspace(query.cwd);
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
    fetcher: async (read) =>
      (await pageData(read.client.skills.listLibrary()).catch(emptyListIfUngated)).map((s) => ({
        name: s.name,
        description: s.description ?? "",
        lifecycle: s.lifecycle,
      })),
  });
  contribute({
    key: WORKSPACE_SKILL_PROPOSALS_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_SKILL_PROPOSALS_KEY, params);
      const resources = await read.workspace(query.cwd);
      return (await pageData(resources.skills.listProposals()).catch(emptyListIfUngated)).map(
        (p) => ({
          workspace: resources.ref.path,
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
    fetcher: async (read) =>
      (await pageData(read.client.tools.list())).map((t) => ({
        name: t.name,
        description: t.description ?? "",
        parameters: t.parameters ?? {},
        safetyClass: t.safetyClass,
      })),
  });
  contribute({
    key: WORKSPACE_KNOWLEDGE_KEY,
    fetcher: async (read, params) => {
      const resources = await read.workspace(optionalParams<WorkspaceKnowledgeQuery>(params)?.cwd);
      return (await pageData(resources.knowledge.list()).catch(emptyListIfUngated)).map((m) => ({
        scope: m.scope,
        content: m.content,
        revision: m.revision,
        updatedAt: m.updatedAt,
      }));
    },
  });
  contribute({
    key: WORKSPACE_AGENT_MEMORY_KEY,
    fetcher: async (read, params) => {
      const q = requiredParams<AgentMemoryQuery>(WORKSPACE_AGENT_MEMORY_KEY, params);
      if (!runtimeCapability("agentMemory")) return [];
      const result =
        q.scope === "user"
          ? await read.client.agentMemory.list({ scope: "user" })
          : await read.workspace(q.cwd).then((resources) => resources.agentMemory.list());
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
    key: WORKSPACE_AGENT_DOCS_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<WorkspaceCatalogQuery>(WORKSPACE_AGENT_DOCS_KEY, params);
      const resources = await read.workspace(query.cwd);
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
    fetcher: async (read) => {
      const enabled = (await pageData(read.client.providers.list())).filter(
        (p) => p.apiKeyMasked !== "",
      );
      // Runtime owns remote-discovery fallback (for example, an offline
      // endpoint falls back to its static catalog). A rejected models.list is
      // therefore a transport / protocol / service failure, not an empty model
      // catalog; preserve it so consumers can render the failure honestly.
      const lists = await Promise.all(enabled.map((p) => pageData(read.client.models.list(p.id))));
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
    fetcher: async (read) =>
      (await pageData(read.client.providers.list())).map((p) => ({
        id: p.id,
        baseUrl: p.baseUrl ?? "",
        apiKeyMasked: p.apiKeyMasked,
        keySource: p.keySource,
        requiresBaseUrl: p.requiresBaseUrl,
        embeddingCapable: p.embeddingCapable,
        defaultEmbeddingModel: p.defaultEmbeddingModel,
      })),
  });
  contribute({
    key: APPROVAL_MODE_KEY,
    fetcher: async (read) => (await read.client.approval.getMode()).mode,
  });
  contribute({
    key: UTILITY_ROLE_KEY,
    fetcher: (read) => read.client.models.getUtilityRole(),
  });
  contribute({
    key: EMBEDDING_ROLE_KEY,
    fetcher: (read) => read.client.models.getEmbeddingRole(),
  });
  contribute({
    key: CODEBASE_STATUS_KEY,
    fetcher: (read, params) => {
      if (!runtimeCapability("codebase")) {
        return Promise.resolve({ state: "none" as const, fileCount: 0, chunkCount: 0 });
      }
      return read
        .workspace(optionalParams<CodebaseStatusQuery>(params)?.cwd)
        .then((resources) => resources.codebase.status());
    },
  });
  contribute({
    key: APPROVAL_RULES_KEY,
    fetcher: async (read, params) => {
      const query = requiredParams<ApprovalRulesQuery>(APPROVAL_RULES_KEY, params);
      return (await read.client.approval.listRules(asSessionId(query.sessionId))).rules;
    },
  });
  contribute({
    key: HOOKS_KEY,
    fetcher: async (read, params) =>
      (await read.workspace(optionalParams<HooksQuery>(params)?.cwd)).hooks.list(),
  });
  contribute({
    key: SCHEDULES_KEY,
    fetcher: async (read) => {
      if (!runtimeCapability("schedules")) return [];
      return (await read.client.schedules.list().autoPagingToArray()).map(scheduleConfig);
    },
  });
  contribute({
    key: RECIPES_KEY,
    fetcher: async (read, params) => {
      const resources = await read.workspace(optionalParams<RecipesQuery>(params)?.cwd);
      return pageData(resources.recipes.list()).catch(emptyListIfUngated);
    },
  });
  contribute({
    key: WORKSPACE_LIST_FILES_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceListFilesQuery>(
        WORKSPACE_LIST_FILES_KEY,
        params,
      );
      const resources = await read.workspace(cwd);
      return (await resources.files.list(query, read.signal).autoPagingToArray()).map((e) => ({
        path: e.path,
        name: e.name,
        type: e.type,
        sizeBytes: e.sizeBytes,
      }));
    },
  });
  contribute({
    key: WORKSPACE_READ_FILE_KEY,
    fetcher: async (read, params) => {
      const { cwd, ...query } = requiredParams<WorkspaceReadFileQuery>(
        WORKSPACE_READ_FILE_KEY,
        params,
      );
      const r = await (await read.workspace(cwd)).files.read(query);
      return { content: r.content, totalLines: r.totalLines, truncated: r.truncated };
    },
  });
}
