import type { WorkspaceGrepMatch } from "./workspaceData";

export const WORKSPACE_SEARCH_MATCH_LIMIT = 200;

export interface WorkspaceSearchResult {
  matches: readonly WorkspaceGrepMatch[];
  total: number;
}

export interface WorkspaceSearchMatchGroup {
  path: string;
  matches: WorkspaceGrepMatch[];
  matchCount: number;
}

export interface WorkspaceSearchViewModel {
  groups: WorkspaceSearchMatchGroup[];
  totalCount: number;
  shownCount: number;
  overflowCount: number;
  hasResult: boolean;
}

export function workspaceSearchViewModel(
  result: WorkspaceSearchResult | null | undefined,
): WorkspaceSearchViewModel {
  const matches = result?.matches ?? [];
  const groups = groupSearchMatchesByFile(matches);
  const totalCount = result?.total ?? 0;

  return {
    groups,
    totalCount,
    shownCount: matches.length,
    overflowCount: Math.max(0, totalCount - matches.length),
    hasResult: result != null,
  };
}

export function workspaceSearchSubtext({
  hasResult,
  totalCount,
}: Pick<WorkspaceSearchViewModel, "hasResult" | "totalCount">): string | undefined {
  if (!hasResult) {
    return undefined;
  }
  return `${totalCount} matches`;
}

function groupSearchMatchesByFile(
  matches: readonly WorkspaceGrepMatch[],
): WorkspaceSearchMatchGroup[] {
  const groups = new Map<string, WorkspaceGrepMatch[]>();
  for (const match of matches) {
    const list = groups.get(match.path);
    if (list) {
      list.push(match);
    } else {
      groups.set(match.path, [match]);
    }
  }

  return Array.from(groups, ([path, groupMatches]) => ({
    path,
    matches: groupMatches,
    matchCount: groupMatches.length,
  }));
}
