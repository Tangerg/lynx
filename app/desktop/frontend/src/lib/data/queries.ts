// React Query hooks for the cached side panels (sessions, files, diff,
// terminal, etc.). Each hook owns one query key + return type. The
// actual fetcher comes from the plugin data-provider registry
// (`lookupDataProvider(key)`); built-in defaults are registered by
// `lyra.builtin.default-data`, but a user plugin can replace any of
// them (fixture data, IPC, in-memory mock, …).

import type { UseQueryResult } from "@tanstack/react-query";
import { useQuery } from "@tanstack/react-query";
import { lookupDataProvider } from "@/plugins/sdk";

// ---- API response shapes ---------------------------------------------------
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
  time: string;
}

export interface SidebarProject {
  id: string;
  name: string;
  branch: string;
  active?: boolean;
}

export interface FileChange {
  path: string;
  change: "add" | "mod" | "del";
  added: number;
  removed: number;
}

export interface MCPServer {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
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
}

export interface ProviderInfo {
  id: string;
  type: string;
  baseUrl: string;
  apiKeyMasked: string;
}

export interface TermLine {
  kind: "prompt" | "cmd" | "out" | "err" | "warn" | "mute" | "ok";
  text: string;
}

export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "ctx"; l: number; r: number; code: string }
  | { type: "add"; r: number; code: string }
  | { type: "del"; l: number; code: string };

export interface GrepMatch {
  path: string;
  match: string;
}

export interface FileLine {
  ln: string; // line number or marker like "···"
  code: string; // already-rendered HTML
  muted?: boolean;
}

// Shape returned by /grep — matches plus a "more matches" total.
export interface GrepResult {
  matches: GrepMatch[];
  total: number;
}

// Shared options — these resources rarely change for the mock, so we cache
// aggressively. Real backends might choose shorter staleTime.
const STATIC = {
  staleTime: 5 * 60_000,
  refetchOnWindowFocus: false as const,
};

function resolve<T>(key: string): () => Promise<T> {
  return () => {
    const fetcher = lookupDataProvider<T>(key);
    if (!fetcher) {
      return Promise.reject(new Error(`No data provider registered for key "${key}"`));
    }
    return fetcher();
  };
}

// One hook per cached side-panel resource. The query key and the
// data-provider key are the same string, passed once — no chance of the
// two drifting apart (a real bug class with the old per-hook literals).
function makeDataQuery<T>(key: string): () => UseQueryResult<T> {
  return () => useQuery({ queryKey: [key], queryFn: resolve<T>(key), ...STATIC });
}

export const useSessions = makeDataQuery<SidebarSession[]>("sessions");
export const useProjects = makeDataQuery<SidebarProject[]>("projects");
export const useFilesChanged = makeDataQuery<FileChange[]>("files-changed");
export const useDiff = makeDataQuery<DiffRow[]>("diff");
export const useTerminal = makeDataQuery<TermLine[]>("terminal");
export const useGrep = makeDataQuery<GrepResult>("grep");
export const useFileHead = makeDataQuery<FileLine[]>("file-head");
export const useMCPServers = makeDataQuery<MCPServer[]>("mcp-servers");
export const useSkills = makeDataQuery<WorkspaceSkill[]>("skills");
export const useAgentDocs = makeDataQuery<WorkspaceAgentDoc[]>("agent-docs");
export const useModels = makeDataQuery<SelectableModel[]>("models");
export const useProviders = makeDataQuery<ProviderInfo[]>("providers");
