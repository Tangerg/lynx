// React Query hooks for the cached side panels (sessions, files, diff,
// etc.). Each hook owns one query key + return type. The
// actual fetcher comes from the plugin data-provider registry
// (`lookupDataProvider(key)`); built-in defaults are registered by
// `lyra.builtin.default-data`, but a user plugin can replace any of
// them (fixture data, IPC, in-memory mock, …).

import type { UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { lookupDataProvider } from "@/plugins/sdk";
import { queryClient } from "./queryClient";

//
// Declared here (the data fetcher) rather than in the rendering
// components so neither state/ nor lib/ has to import upward into
// components/ for type-only metadata. Components import these types
// from `@/lib/data/queries` when they need to type a row prop.

export interface SidebarSession {
  id: string;
  title: string;
  status: "running" | "waiting" | "idle";
  model: string;
  cwd?: string; // session working directory — absent on 1:1 placeholder rows (PanelHeader)
  cwdMissing?: boolean; // cwd lost on disk → agent degrades to plain chat (relocate to fix)
  /** Cumulative session usage (wire Session.usage). costUsd stays absent
   *  when the model isn't in the pricing table — never fabricate 0. */
  usage?: { inputTokens?: number; outputTokens?: number; costUsd?: number };
  time: string;
}

export interface SidebarProject {
  id: string; // = Project.cwd (the wire identity)
  name: string;
  branch: string;
  sessionCount: number;
  cwdMissing?: boolean; // directory gone from disk (relocate/restore to fix)
  active?: boolean;
}

export interface FileChange {
  path: string;
  change: "add" | "mod" | "del";
  added?: number; // absent for binary files (never a fabricated 0)
  removed?: number;
  binary?: boolean;
}

export interface MCPServer {
  id: string;
  name: string;
  desc: string;
  tools: number;
  // Mirrors the wire McpStatus lifecycle (AUX_API §5.1) — reconnect flows
  // push connecting → (connected | failed | needsAuth) through the
  // workspace event channel, and the row binds its loading state to it.
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";
  /** Why the server is `failed` — shown in the row tooltip. */
  errorDetail?: string;
  icon: string;
}

// One MCP tool row for the expanded server detail (workspace.mcp.listTools).
export interface McpToolInfo {
  name: string;
  description: string;
}
export interface McpToolsQuery {
  server: string;
}

// One built-in (non-MCP) tool from the runtime's catalog (tools.list).
export interface BuiltinToolInfo {
  name: string;
  description: string;
  safetyClass?: string; // "safe" | "write" | "exec" | "network" …
}

export interface WorkspaceSkill {
  name: string;
  description: string;
  source: string;
}

export interface WorkspaceAgentDoc {
  path: string;
  title: string;
  scope: "cwd" | "projectRoot" | "home";
}

export interface SelectableModel {
  id: string;
  provider: string;
  label: string;
  /** Accepts image input (Model.capabilities.multimodal) — gates the composer's
   *  attach affordance. */
  multimodal: boolean;
}

export interface ProviderInfo {
  id: string;
  baseUrl: string;
  apiKeyMasked: string;
}

// workspace.getDiff params + result (AUX_API §2.3) — structured rows only;
// the raw-patch format is for export flows and gets its own hook when needed.
export interface DiffQuery {
  cwd?: string; // default = serve dir; pass the active session's cwd
  path?: string; // omit = whole working tree
  mode?: "worktree" | "base"; // default worktree (includes untracked)
  limit?: number; // row cap; server truncates at file boundaries
}

export interface FileChangesQuery {
  cwd?: string; // default = serve dir; pass the active session's cwd
}
export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };
export interface FileDiff {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string; // only on renames
  added?: number; // absent for binary files
  removed?: number;
  binary?: boolean;
  rows: DiffRow[]; // [] for binary files
}
export interface WorkspaceDiff {
  files: FileDiff[];
  truncated?: boolean;
}

// workspace.grep params + result (API.md §7.5). `total` may exceed
// matches.length — that's the truncation signal ("narrow the query"), never
// assume the two are equal.
export interface GrepQuery {
  query: string; // regex
  cwd?: string; // default = serve dir; pass the active session's cwd
  path?: string; // optional sub-path jail under cwd
  limit?: number; // default 100 server-side
}
export interface GrepMatch {
  path: string;
  lineNumber: number;
  text: string;
}
export interface GrepResult {
  matches: GrepMatch[];
  total: number;
}

// memory.* (features.memory gated) — the LYRA.md memory files the runtime
// reads into the agent's context, one per scope. Content rides along so the
// Memory panel can edit in place (memory.update writes whole-file).
export interface MemoryQuery {
  cwd?: string; // default = serve dir; pass the active session's cwd
}
export interface MemoryEntryInfo {
  scope: "cwd" | "projectRoot" | "home";
  path: string;
  content: string;
  updatedAt?: string;
}

// workspace.getFileHead params + row (API.md §7.5) — plain text, 1-based
// line numbers; highlighting is the renderer's job.
export interface FileHeadQuery {
  path: string; // relative to cwd
  cwd?: string; // default = serve dir; pass the active session's cwd
  lines?: number; // default 200 server-side
}
export interface FileLine {
  lineNumber: number;
  text: string;
}

// approval.* (B9) — the runtime's global approval stance + the persistent
// fine-grained "remember this decision" rules. Mode is mutated in the Approvals
// pane (lib/agent/approvalConfig); both keys invalidate on mutation.
export type ApprovalModeValue = "plan" | "safe" | "balanced" | "yolo";
export interface ApprovalRulesQuery {
  sessionId: string;
}
export interface ApprovalRuleInfo {
  id: string;
  scope: "session" | "project" | "global";
  tool: string;
  subject?: string;
  dir?: string;
  decision: "allow" | "deny";
}

// workspace.listFiles / readFile (B8, 613) — the file-tree browser + file
// viewer. Lazy: the tree fetches one directory level per expand. Not gated
// (basic reads, like getFileHead); a pre-B8 runtime simply errors the query.
export interface ListFilesQuery {
  cwd?: string;
  path?: string; // directory to list, relative to cwd (omit = cwd root)
  recursive?: boolean; // flat subtree file list (the @file source) vs one level (tree)
  limit?: number;
}
export interface FileEntryInfo {
  path: string; // relative to cwd
  name: string;
  type: "file" | "dir" | "symlink";
  sizeBytes?: number;
}
export interface ReadFileQuery {
  path: string;
  cwd?: string;
}
export interface FileContentInfo {
  content: string;
  totalLines: number;
  truncated?: boolean;
}

// Shared options — these resources rarely change for the mock, so we cache
// aggressively. Real backends might choose shorter staleTime.
const STATIC = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

function resolve<T, P = void>(key: string, params?: P): () => Promise<T> {
  return () => {
    const fetcher = lookupDataProvider<T, P>(key);
    if (!fetcher) {
      return Promise.reject(new Error(`No data provider registered for key "${key}"`));
    }
    return fetcher(params);
  };
}

// One hook per cached side-panel resource. The query key and the
// data-provider key are the same string, passed once — no chance of the
// two drifting apart (a real bug class with the old per-hook literals).
function makeDataQuery<T>(key: string): () => UseQueryResult<T> {
  return () => useQuery({ queryKey: [key], queryFn: resolve<T>(key), ...STATIC });
}

// Parameterized variant — params join the query key (each distinct params
// object caches independently) and flow into the provider. `undefined`
// params disables the query (the caller has nothing to ask yet).
function makeParamDataQuery<P, T>(key: string): (params: P | undefined) => UseQueryResult<T> {
  return (params) =>
    useQuery({
      queryKey: [key, params],
      queryFn: resolve<T, P>(key, params),
      enabled: params !== undefined,
      ...STATIC,
    });
}

// Keys that are also INVALIDATED outside this file (lib/agent mutation
// hooks, the workspace-events plugin). Named so the literal exists in
// exactly one place — the same no-drift argument as passing the key once
// into makeDataQuery. Keys only ever read stay inline below.
export const SESSIONS_KEY = "sessions";
export const PROJECTS_KEY = "projects";
export const PROVIDERS_KEY = "providers";
export const MODELS_KEY = "models";
export const FILES_CHANGED_KEY = "files-changed";
export const DIFF_KEY = "diff";
export const SKILLS_KEY = "skills";
export const MCP_SERVERS_KEY = "mcp-servers";
export const MCP_TOOLS_KEY = "mcp-tools";
export const MEMORY_KEY = "memory";
export const APPROVAL_MODE_KEY = "approval-mode";
export const APPROVAL_RULES_KEY = "approval-rules";

export const useSessions = makeDataQuery<SidebarSession[]>(SESSIONS_KEY);
export const useProjects = makeDataQuery<SidebarProject[]>(PROJECTS_KEY);

/** The server's session set changed (create / delete / rename / fork /
 *  relocate / import) — refetch the sidebar list. cwd-affecting mutations
 *  pass `projects: true`: a new cwd can mint (or retire) a project node. */
export function invalidateSessions(opts?: { projects?: boolean }): Promise<void> {
  const sessions = queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
  if (!opts?.projects) return sessions;
  return Promise.all([sessions, queryClient.invalidateQueries({ queryKey: [PROJECTS_KEY] })]).then(
    () => undefined,
  );
}
export const useFilesChanged = makeParamDataQuery<FileChangesQuery, FileChange[]>(
  FILES_CHANGED_KEY,
);
export const useDiff = makeParamDataQuery<DiffQuery, WorkspaceDiff>(DIFF_KEY);
export const useGrep = makeParamDataQuery<GrepQuery, GrepResult>("grep");
export const useFileHead = makeParamDataQuery<FileHeadQuery, FileLine[]>("file-head");
export const useMCPServers = makeDataQuery<MCPServer[]>(MCP_SERVERS_KEY);
export const useBuiltinTools = makeDataQuery<BuiltinToolInfo[]>("builtin-tools");
export const useMCPTools = makeParamDataQuery<McpToolsQuery, McpToolInfo[]>(MCP_TOOLS_KEY);
export const useSkills = makeDataQuery<WorkspaceSkill[]>(SKILLS_KEY);
export const useMemory = makeParamDataQuery<MemoryQuery, MemoryEntryInfo[]>(MEMORY_KEY);
export const useAgentDocs = makeDataQuery<WorkspaceAgentDoc[]>("agent-docs");
export const useModels = makeDataQuery<SelectableModel[]>(MODELS_KEY);
export const useProviders = makeDataQuery<ProviderInfo[]>(PROVIDERS_KEY);
export const useApprovalMode = makeDataQuery<ApprovalModeValue>(APPROVAL_MODE_KEY);
export const useApprovalRules = makeParamDataQuery<ApprovalRulesQuery, ApprovalRuleInfo[]>(
  APPROVAL_RULES_KEY,
);
export const useListFiles = makeParamDataQuery<ListFilesQuery, FileEntryInfo[]>("list-files");
export const useReadFile = makeParamDataQuery<ReadFileQuery, FileContentInfo>("read-file");
