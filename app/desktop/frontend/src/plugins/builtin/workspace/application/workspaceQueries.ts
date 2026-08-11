import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import { useRecipes } from "@/plugins/builtin/chat/recipes/public/queries";

export interface WorkspaceProjectSummary {
  id: string;
  name: string;
  sessionCount: number;
  cwdMissing?: boolean;
}

export interface WorkspaceFileChange {
  path: string;
  change: "add" | "mod" | "del";
  added?: number;
  removed?: number;
  binary?: boolean;
}

export interface BuiltinToolSummary {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  safetyClass?: string;
}

export interface WorkspaceSkill {
  name: string;
  description: string;
  /** Which library it was discovered in — project-local or the user's own. */
  scope: "project" | "user";
}

// Workspace catalog reads are keyed by the session workspace. Keeping the
// scope in the query identity prevents a catalog from one open project being
// reused after the user switches to another.
export interface WorkspaceCatalogQuery {
  cwd?: string;
}

// One entry in the global self-authored skill library (skills.library.list),
// tagged with its curator lifecycle. Distinct from WorkspaceSkill (the agent's
// project+global discovery view): this is the management surface, which also
// lists archived skills.
export interface ManagedSkill {
  name: string;
  description: string;
  lifecycle: "active" | "archived";
}

// One skill proposal awaiting offline review (skills.proposals.list).
// name+revision+scope is the handle an approve/reject decision carries;
// origin and sourceSession are the provenance shown to the reviewer, and
// `revises` says an approval would overwrite a Skill that already loads.
export interface SkillProposal {
  /** The exact workspace against which this immutable proposal was listed. */
  workspace: string;
  name: string;
  revision: string;
  scope: "project" | "user";
  description: string;
  instructions: string;
  origin: "requested" | "mined";
  revises: boolean;
  sourceSession: string;
}

// The (scope, cwd) key an agentMemory read is bound to. The user scope ignores
// cwd; the project scope resolves it to the session's project.
export interface AgentMemoryQuery {
  scope: "project" | "user";
  cwd?: string;
}

// One addressable agent-memory item (agentMemory.list). status is
// active | pending (pending items await review); origin is auto (mined) | user
// (authored). Distinct from WorkspaceKnowledgeEntry (the LYRA.md file cascade).
export interface AgentMemoryEntry {
  id: string;
  scope: "project" | "user";
  content: string;
  origin: "auto" | "user";
  status: "active" | "pending";
  pinned: boolean;
  sessionId: string;
  day: string;
  createdAt: string;
  updatedAt: string;
}

/** Where a project-level agent doc lives: the session cwd, the project root, or
 *  the user's home. Shared by the knowledge files and the AGENTS.md docs. */
export type WorkspaceKnowledgeScope = "cwd" | "projectRoot" | "home";

export interface WorkspaceAgentDoc {
  path: string;
  title: string;
  scope: WorkspaceKnowledgeScope;
}

export interface WorkspaceDiffQuery {
  cwd?: string;
  path?: string;
  mode?: "worktree" | "base";
  limit?: number;
}

export interface WorkspaceFileChangesQuery {
  cwd?: string;
}

export type WorkspaceDiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

export interface WorkspaceFileDiff {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: boolean;
  rows: WorkspaceDiffRow[];
}

export interface WorkspaceDiff {
  files: WorkspaceFileDiff[];
  truncated?: boolean;
}

export interface WorkspaceGrepQuery {
  query: string;
  cwd?: string;
  path?: string;
  limit?: number;
}

export interface WorkspaceGrepMatch {
  path: string;
  lineNumber: number;
  text: string;
}

export interface WorkspaceGrepResult {
  matches: WorkspaceGrepMatch[];
  total: number;
}

export interface WorkspaceKnowledgeQuery {
  cwd?: string;
}

export interface WorkspaceKnowledgeEntry {
  scope: WorkspaceKnowledgeScope;
  content: string;
  updatedAt?: string;
}

export interface WorkspaceFileHeadQuery {
  path: string;
  cwd?: string;
  lines?: number;
}

export interface WorkspaceFileLine {
  lineNumber: number;
  text: string;
}

export interface WorkspaceListFilesQuery {
  cwd?: string;
  path?: string;
  recursive?: boolean;
  limit?: number;
}

export interface WorkspaceFileEntry {
  path: string;
  name: string;
  type: "file" | "dir" | "symlink";
  sizeBytes?: number;
}

export interface WorkspaceReadFileQuery {
  path: string;
  cwd?: string;
}

export interface WorkspaceFileContent {
  content: string;
  totalLines: number;
  truncated?: boolean;
}

export const WORKSPACE_PROJECTS_KEY = "projects";
export const WORKSPACE_FILES_CHANGED_KEY = "files-changed";
export const WORKSPACE_DIFF_KEY = "diff";
export const WORKSPACE_SKILLS_KEY = "skills";
export const WORKSPACE_MANAGED_SKILLS_KEY = "managed-skills";
export const WORKSPACE_SKILL_PROPOSALS_KEY = "skill-proposals";
export const WORKSPACE_AGENT_MEMORY_KEY = "agent-memory";
export const WORKSPACE_KNOWLEDGE_KEY = "knowledge";
export const WORKSPACE_BUILTIN_TOOLS_KEY = "builtin-tools";
export const WORKSPACE_GREP_KEY = "grep";
export const WORKSPACE_FILE_HEAD_KEY = "file-head";
export const WORKSPACE_AGENT_DOCS_KEY = "agent-docs";
export const WORKSPACE_LIST_FILES_KEY = "list-files";
export const WORKSPACE_READ_FILE_KEY = "read-file";

export const useWorkspaceProjects =
  createDataQuery<WorkspaceProjectSummary[]>(WORKSPACE_PROJECTS_KEY);
export const useWorkspaceFileChanges = createParameterizedDataQuery<
  WorkspaceFileChangesQuery,
  WorkspaceFileChange[]
>(WORKSPACE_FILES_CHANGED_KEY);
export const useWorkspaceDiff = createParameterizedDataQuery<WorkspaceDiffQuery, WorkspaceDiff>(
  WORKSPACE_DIFF_KEY,
);
export const useWorkspaceGrep = createParameterizedDataQuery<
  WorkspaceGrepQuery,
  WorkspaceGrepResult
>(WORKSPACE_GREP_KEY);
export const useWorkspaceFileHead = createParameterizedDataQuery<
  WorkspaceFileHeadQuery,
  WorkspaceFileLine[]
>(WORKSPACE_FILE_HEAD_KEY);
export const useWorkspaceBuiltinTools = createDataQuery<BuiltinToolSummary[]>(
  WORKSPACE_BUILTIN_TOOLS_KEY,
);
export const useWorkspaceSkills = createParameterizedDataQuery<
  WorkspaceCatalogQuery,
  WorkspaceSkill[]
>(WORKSPACE_SKILLS_KEY);
export const useManagedSkills = createDataQuery<ManagedSkill[]>(WORKSPACE_MANAGED_SKILLS_KEY);
export const useSkillProposals = createParameterizedDataQuery<
  WorkspaceCatalogQuery,
  SkillProposal[]
>(WORKSPACE_SKILL_PROPOSALS_KEY);
export const useAgentMemory = createParameterizedDataQuery<AgentMemoryQuery, AgentMemoryEntry[]>(
  WORKSPACE_AGENT_MEMORY_KEY,
);
export const useWorkspaceKnowledge = createParameterizedDataQuery<
  WorkspaceKnowledgeQuery,
  WorkspaceKnowledgeEntry[]
>(WORKSPACE_KNOWLEDGE_KEY);
export const useWorkspaceAgentDocs = createParameterizedDataQuery<
  WorkspaceCatalogQuery,
  WorkspaceAgentDoc[]
>(WORKSPACE_AGENT_DOCS_KEY);
export const useWorkspaceListFiles = createParameterizedDataQuery<
  WorkspaceListFilesQuery,
  WorkspaceFileEntry[]
>(WORKSPACE_LIST_FILES_KEY);
export const useWorkspaceReadFile = createParameterizedDataQuery<
  WorkspaceReadFileQuery,
  WorkspaceFileContent
>(WORKSPACE_READ_FILE_KEY);
export const useWorkspaceRecipes = useRecipes;
