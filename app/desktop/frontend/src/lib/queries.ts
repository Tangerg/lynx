// React Query hooks — Phase 11.
//
// Each hook owns one query key + return type. The actual fetcher comes
// from the plugin data-provider registry (`lookupDataProvider(key)`); the
// built-in `lyra.builtin.default-data` plugin registers the original
// HTTP-backed fetchers, but a user plugin can replace any of them (e.g.
// fixture data, IPC, in-memory mock).

import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { lookupDataProvider } from "@/plugins/sdk";
import type { SidebarProject, SidebarSession } from "@/components/sidebar/types";
import type { FileChange, MCPServer } from "@/components/inspector/types";
import type { DiffRow, FileLine, GrepMatch, TermLine } from "@/components/tools/previews";

// Shape returned by /grep — matches plus a "more matches" total.
export type GrepResult = { matches: GrepMatch[]; total: number };

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
      return Promise.reject(
        new Error(`No data provider registered for key "${key}"`),
      );
    }
    return fetcher();
  };
}

export function useSessions(): UseQueryResult<SidebarSession[]> {
  return useQuery({ queryKey: ["sessions"], queryFn: resolve<SidebarSession[]>("sessions"), ...STATIC });
}

export function useProjects(): UseQueryResult<SidebarProject[]> {
  return useQuery({ queryKey: ["projects"], queryFn: resolve<SidebarProject[]>("projects"), ...STATIC });
}

export function useFilesChanged(): UseQueryResult<FileChange[]> {
  return useQuery({ queryKey: ["files-changed"], queryFn: resolve<FileChange[]>("files-changed"), ...STATIC });
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
  return useQuery({ queryKey: ["file-head"], queryFn: resolve<FileLine[]>("file-head"), ...STATIC });
}

export function useMCPServers(): UseQueryResult<MCPServer[]> {
  return useQuery({ queryKey: ["mcp-servers"], queryFn: resolve<MCPServer[]>("mcp-servers"), ...STATIC });
}
