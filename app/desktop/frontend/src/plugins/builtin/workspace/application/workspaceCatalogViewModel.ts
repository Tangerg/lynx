import type { WorkspaceKnowledgeEntry } from "./workspaceQueries";
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
  revision: string;
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
      revision: entry.revision,
      updatedAt: entry.updatedAt,
    })),
  );
}

function knowledgePath(scope: WorkspaceKnowledgeScope): string {
  if (scope === "cwd") return "SCOPEAPP.md";
  if (scope === "projectRoot") return "project/SCOPEAPP.md";
  return "~/.scopeapp/SCOPEAPP.md";
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
