import type { WorkspaceKnowledgeEntry } from "./workspaceQueries";
import type { CodebaseSearchHit } from "./ports/codebaseGateway";
import type {
  WorkspaceAgentDoc,
  WorkspaceKnowledgeScope,
  WorkspaceSkill,
} from "./workspaceQueries";

export interface WorkspaceCatalogViewModel<Row> {
  rows: Row[];
  count: number;
  enabled: boolean;
  isEmpty: boolean;
}

export interface WorkspaceKnowledgeRowViewModel {
  id: string;
  scope: WorkspaceKnowledgeScope;
  scopeLabelKey: string;
  path: string;
  content: string;
  updatedAt?: string;
}

export interface WorkspaceSkillRowViewModel {
  id: string;
  name: string;
  description: string;
  scope: "project" | "user";
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
  scopeLabelKey: string;
}

export interface CodebaseStatusProjection {
  state: "ready" | "indexing" | "error" | "none";
  fileCount: number;
  chunkCount: number;
}

export interface CodebaseSearchRowViewModel {
  id: string;
  path: string;
  startLine: number;
  pathRange: string;
  score: string;
  snippet: string;
}

export interface CodebaseSearchViewModel {
  rows: CodebaseSearchRowViewModel[];
  isEmpty: boolean;
}

// The scope words live in the catalogs; this maps a scope to its key.
const SCOPE_LABEL_KEY: Record<WorkspaceKnowledgeScope, string> = {
  cwd: "knowledge.scope.cwd",
  projectRoot: "knowledge.scope.projectRoot",
  home: "knowledge.scope.home",
};

function catalog<Row>(rows: Row[], enabled = true): WorkspaceCatalogViewModel<Row> {
  return {
    rows,
    count: rows.length,
    enabled,
    isEmpty: rows.length === 0,
  };
}

/** The catalog key for a knowledge scope; an unknown scope reads as itself. */
export function scopeLabelKey(scope: string): string {
  return SCOPE_LABEL_KEY[scope as WorkspaceKnowledgeScope] ?? scope;
}

export function workspaceKnowledgeViewModel(
  entries: readonly WorkspaceKnowledgeEntry[],
  enabled: boolean,
): WorkspaceCatalogViewModel<WorkspaceKnowledgeRowViewModel> {
  if (!enabled) {
    return catalog([], false);
  }

  return catalog(
    entries.map((entry) => ({
      id: entry.scope,
      scope: entry.scope,
      scopeLabelKey: scopeLabelKey(entry.scope),
      path: knowledgePath(entry.scope),
      content: entry.content,
      updatedAt: entry.updatedAt,
    })),
  );
}

function knowledgePath(scope: WorkspaceKnowledgeScope): string {
  if (scope === "cwd") return "LYRA.md";
  if (scope === "projectRoot") return "project/LYRA.md";
  return "~/.lyra/LYRA.md";
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
      scope: skill.scope,
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
      scopeLabelKey: scopeLabelKey(doc.scope),
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
      path: hit.path,
      startLine: hit.startLine,
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
