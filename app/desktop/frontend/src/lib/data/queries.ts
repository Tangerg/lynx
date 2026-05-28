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

export function useSessions(): UseQueryResult<SidebarSession[]> {
  return useQuery({
    queryKey: ["sessions"],
    queryFn: resolve<SidebarSession[]>("sessions"),
    ...STATIC,
  });
}

export function useProjects(): UseQueryResult<SidebarProject[]> {
  return useQuery({
    queryKey: ["projects"],
    queryFn: resolve<SidebarProject[]>("projects"),
    ...STATIC,
  });
}

export function useFilesChanged(): UseQueryResult<FileChange[]> {
  return useQuery({
    queryKey: ["files-changed"],
    queryFn: resolve<FileChange[]>("files-changed"),
    ...STATIC,
  });
}

export function useDiff(): UseQueryResult<DiffRow[]> {
  return useQuery({ queryKey: ["diff"], queryFn: resolve<DiffRow[]>("diff"), ...STATIC });
}

export function useTerminal(): UseQueryResult<TermLine[]> {
  return useQuery({ queryKey: ["terminal"], queryFn: resolve<TermLine[]>("terminal"), ...STATIC });
}

export function useGrep(): UseQueryResult<GrepResult> {
  return useQuery({ queryKey: ["grep"], queryFn: resolve<GrepResult>("grep"), ...STATIC });
}

export function useFileHead(): UseQueryResult<FileLine[]> {
  return useQuery({
    queryKey: ["file-head"],
    queryFn: resolve<FileLine[]>("file-head"),
    ...STATIC,
  });
}

export function useMCPServers(): UseQueryResult<MCPServer[]> {
  return useQuery({
    queryKey: ["mcp-servers"],
    queryFn: resolve<MCPServer[]>("mcp-servers"),
    ...STATIC,
  });
}
