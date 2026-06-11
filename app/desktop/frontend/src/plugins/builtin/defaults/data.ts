// `lyra.builtin.default-data` — the data providers behind every cached
// side-panel hook in `lib/data/queries`. Sibling file for the same reason
// as `commands.ts`: substantially bigger than the other defaults.
//
// Cutover mappers: most side-panel keys ride the JSON-RPC stack. Where the
// protocol shape differs from the sidebar row, we map it down here (the
// protocol intentionally omits client-side presentation like the MCP icon
// — see API.md §6.5).

import type {
  DiffQuery,
  FileChange as SidebarFileChange,
  FileHeadQuery,
  GrepQuery,
  MCPServer as SidebarMCPServer,
  SidebarProject,
  SidebarSession,
  WorkspaceDiff,
} from "@/lib/data/queries";
import type {
  McpServer as RpcMCPServer,
  Project as RpcProject,
  Session,
  WorkspaceFileChange as RpcFileChange,
} from "@/rpc";
import { api } from "@/lib/data/http";
import { MODELS_KEY, PROVIDERS_KEY, SESSIONS_KEY } from "@/lib/data/queries";
import { getContainer } from "@/main/container";
import { errorType, RpcError } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";

// `sessions` — protocol Session is richer than the sidebar row.
function toSidebarSession(s: Session): SidebarSession {
  return {
    id: s.id,
    title: s.title,
    status: s.status,
    model: s.model,
    time: s.updatedAt || s.createdAt,
  };
}

// `mcp-servers` — the protocol MCPServer carries no id/icon (both are
// client-side). Use the MCP name as the stable id and map name → icon;
// status passes through verbatim (the UI shape mirrors the wire lifecycle).
const MCP_ICON: Record<string, string> = {
  Filesystem: "folder",
  Git: "branch",
  Shell: "terminal",
  "Web Search": "globe",
  Linear: "list",
  GitHub: "git",
  Postgres: "tool",
  Slack: "chat",
};
function toSidebarMCPServer(s: RpcMCPServer): SidebarMCPServer {
  return {
    id: s.name,
    name: s.name,
    desc: s.description ?? "",
    tools: s.toolCount ?? 0,
    status: s.status,
    errorDetail: s.error ? (s.error.detail ?? s.error.type) : undefined,
    icon: MCP_ICON[s.name] ?? "tool",
  };
}

// `projects` — v2 Project keys identity on cwd (no opaque id, no active flag).
function toSidebarProject(p: RpcProject): SidebarProject {
  return { id: p.cwd, name: p.name, branch: p.branch ?? "" };
}

// `files-changed` — collapse the five wire statuses into the sidebar's three
// change codes; ± line counts ride along (absent for binary files, AUX_API §2.2).
const FILE_CHANGE: Record<RpcFileChange["status"], SidebarFileChange["change"]> = {
  added: "add",
  untracked: "add",
  modified: "mod",
  renamed: "mod",
  deleted: "del",
};
function toSidebarFileChange(f: RpcFileChange): SidebarFileChange {
  return {
    path: f.path,
    change: FILE_CHANGE[f.status],
    added: f.added,
    removed: f.removed,
    binary: f.binary,
  };
}

// Capability-gated workspace reads (skills / agent docs) return
// capability_not_negotiated when the runtime has the feature off (§9). Treat
// that as "none" (an empty Page) so the view shows its empty state instead of
// an error toast.
function emptyPageIfUngated(err: unknown): { data: never[] } {
  if (err instanceof RpcError && errorType(err.data) === "capability_not_negotiated")
    return { data: [] };
  throw err;
}

// HTTP_KEYS lists the query keys still served over REST GET. Everything else
// rides the JSON-RPC stack now; `terminal` is a stream (#160). Adding a key
// in queries without a provider here makes that hook reject.
const HTTP_KEYS = ["terminal"] as const;

export const defaultData = definePlugin({
  name: "lyra.builtin.default-data",
  version: "1.0.0",
  setup({ host }) {
    const client = () => getContainer().client();

    host.extensions.contribute(DATA_PROVIDER, {
      key: SESSIONS_KEY,
      fetcher: async () => (await client().sessions.list()).data.map(toSidebarSession),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "projects",
      fetcher: async () => (await client().workspace.listProjects()).data.map(toSidebarProject),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "files-changed",
      fetcher: async () =>
        (await client().workspace.listFileChanges()).data.map(toSidebarFileChange),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "mcp-servers",
      // B3 enriched the entry with toolCount/authStatus/error inline
      // (AUX_API §5.1) — no more listServers⨝listTools join; listTools
      // stays for the detail pane (pagination + inputSchema).
      fetcher: async () =>
        (await client().workspace.mcp.listServers()).data.map(toSidebarMCPServer),
    });
    // Parameterized workspace reads — params come from the consumer hook
    // (queries.ts makeParamDataQuery), so each distinct query caches its own
    // entry. Wire shapes match the UI shapes 1:1 (queries.ts re-declares them
    // so components never import @/rpc).
    host.extensions.contribute(DATA_PROVIDER, {
      key: "diff",
      // format is pinned to rows here — the structured form every renderer
      // consumes; raw unified patches are an export concern (AUX_API §2.3).
      fetcher: async (params) => {
        const diff = await client().workspace.getDiff({ ...(params as DiffQuery), format: "rows" });
        return { files: diff.files ?? [], truncated: diff.truncated } satisfies WorkspaceDiff;
      },
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "grep",
      fetcher: (params) => client().workspace.grep(params as GrepQuery),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "file-head",
      fetcher: async (params) =>
        (await client().workspace.getFileHead(params as FileHeadQuery)).lines,
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "skills",
      fetcher: async () =>
        (await client().workspace.listSkills().catch(emptyPageIfUngated)).data.map((s) => ({
          name: s.name,
          description: s.description ?? "",
          source: s.source ?? "",
        })),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: "agent-docs",
      fetcher: async () =>
        (await client().workspace.listAgentDocs().catch(emptyPageIfUngated)).data.map((d) => ({
          path: d.path,
          title: d.title ?? "",
          scope: d.scope,
        })),
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: MODELS_KEY,
      // models.list is now per-provider (an empty `provider` returns []), so
      // aggregate across the ENABLED providers (apiKeyMasked != "" ⇔ key set).
      // Unconfigured providers are catalog-only and can't run, so they'd just
      // litter the picker with dead options — configure one in Settings →
      // Providers to surface its models here.
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
        }));
      },
    });
    host.extensions.contribute(DATA_PROVIDER, {
      key: PROVIDERS_KEY,
      fetcher: async () =>
        (await client().providers.list()).data.map((p) => ({
          id: p.id,
          type: p.type,
          baseUrl: p.baseUrl ?? "",
          apiKeyMasked: p.apiKeyMasked,
        })),
    });

    for (const key of HTTP_KEYS) {
      host.extensions.contribute(DATA_PROVIDER, {
        key,
        fetcher: () => api.get(key).json(),
      });
    }
  },
});
