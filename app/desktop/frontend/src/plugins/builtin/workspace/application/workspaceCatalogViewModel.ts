import type { WorkspaceMemoryEntry } from "./memoryConfig";
import type { CodebaseSearchHit } from "./ports/codebaseGateway";
import type { WorkspaceAgentDoc, WorkspaceScope, WorkspaceSkill } from "./workspaceData";

export interface WorkspaceCatalogViewModel<Row> {
  rows: Row[];
  count: number;
  enabled: boolean;
  isEmpty: boolean;
}

export interface WorkspaceMemoryRowViewModel {
  id: string;
  scope: WorkspaceMemoryEntry["scope"];
  scopeLabel: string;
  path: string;
  content: string;
  updatedAt?: string;
}

export interface WorkspaceSkillRowViewModel {
  id: string;
  name: string;
  description: string;
  source: string;
}

export interface WorkspaceRecipeCatalogEntry {
  name: string;
  description?: string;
  argumentHint?: string;
  scope: string;
  source: string;
}

export interface WorkspaceRecipeRowViewModel {
  id: string;
  command: string;
  description?: string;
  argumentHint?: string;
  scope: string;
}

export interface WorkspaceAgentDocRowViewModel {
  id: string;
  title: string;
  path: string;
  scopeLabel: string;
}

export interface CodebaseStatusProjection {
  state: "ready" | "indexing" | "error" | "none";
  fileCount: number;
  chunkCount: number;
}

export interface CodebaseSearchRowViewModel {
  id: string;
  pathRange: string;
  score: string;
  snippet: string;
}

export interface CodebaseSearchViewModel {
  rows: CodebaseSearchRowViewModel[];
  isEmpty: boolean;
}

const SCOPE_LABEL: Record<WorkspaceScope, string> = {
  cwd: "cwd",
  projectRoot: "project root",
  home: "home",
};

function catalog<Row>(rows: Row[], enabled = true): WorkspaceCatalogViewModel<Row> {
  return {
    rows,
    count: rows.length,
    enabled,
    isEmpty: rows.length === 0,
  };
}

export function scopeLabel(scope: string): string {
  return (SCOPE_LABEL as Record<string, string>)[scope] ?? scope;
}

export function workspaceMemoryViewModel(
  entries: readonly WorkspaceMemoryEntry[],
  enabled: boolean,
): WorkspaceCatalogViewModel<WorkspaceMemoryRowViewModel> {
  if (!enabled) {
    return catalog([], false);
  }

  return catalog(
    entries.map((entry) => ({
      id: `${entry.scope}:${entry.path}`,
      scope: entry.scope,
      scopeLabel: scopeLabel(entry.scope),
      path: entry.path,
      content: entry.content,
      updatedAt: entry.updatedAt,
    })),
  );
}

export function workspaceSkillsViewModel(
  skills: readonly WorkspaceSkill[],
  enabled: boolean,
): WorkspaceCatalogViewModel<WorkspaceSkillRowViewModel> {
  if (!enabled) {
    return catalog([], false);
  }

  return catalog(
    skills.map((skill) => ({
      id: skill.name,
      name: skill.name,
      description: skill.description,
      source: skill.source,
    })),
  );
}

export function workspaceRecipesViewModel(
  recipes: readonly WorkspaceRecipeCatalogEntry[],
): WorkspaceCatalogViewModel<WorkspaceRecipeRowViewModel> {
  return catalog(
    recipes.map((recipe) => ({
      id: `${recipe.source}:${recipe.name}`,
      command: `/${recipe.name}`,
      description: recipe.description,
      argumentHint: recipe.argumentHint,
      scope: recipe.scope,
    })),
  );
}

export function workspaceAgentDocsViewModel(
  docs: readonly WorkspaceAgentDoc[],
): WorkspaceCatalogViewModel<WorkspaceAgentDocRowViewModel> {
  return catalog(
    docs.map((doc) => ({
      id: doc.path,
      title: doc.title || doc.path,
      path: doc.path,
      scopeLabel: scopeLabel(doc.scope),
    })),
  );
}

export function codebaseStatusViewModel(
  status:
    | {
        state?: string;
        fileCount?: number;
        chunkCount?: number;
      }
    | undefined,
): CodebaseStatusProjection {
  return {
    state: codebaseStatusState(status?.state),
    fileCount: status?.fileCount ?? 0,
    chunkCount: status?.chunkCount ?? 0,
  };
}

export function codebaseSearchViewModel(
  hits: readonly CodebaseSearchHit[] | null,
): CodebaseSearchViewModel {
  const rows =
    hits?.map((hit, index) => ({
      id: `${hit.path}:${hit.startLine}:${hit.endLine}:${index}`,
      pathRange: `${hit.path}:${hit.startLine}-${hit.endLine}`,
      score: hit.score.toFixed(2),
      snippet: hit.snippet,
    })) ?? [];

  return {
    rows,
    isEmpty: hits !== null && rows.length === 0,
  };
}

function codebaseStatusState(state: string | undefined): CodebaseStatusProjection["state"] {
  switch (state) {
    case "ready":
    case "indexing":
    case "error":
      return state;
    default:
      return "none";
  }
}
