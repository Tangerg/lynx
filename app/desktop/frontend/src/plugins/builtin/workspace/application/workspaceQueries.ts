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
  safetyClass?: string;
}

export interface WorkspaceSkill {
  name: string;
  description: string;
  source: string;
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

// One agent-mined skill proposal awaiting offline review (skills.drafts.list).
// name+revision is the content-addressed handle a promote/reject decision
// carries; createdBy/sourceSession is the provenance shown to the reviewer.
export interface SkillDraft {
  name: string;
  revision: string;
  description: string;
  createdBy: string;
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
// (authored). Distinct from WorkspaceMemoryEntry (the LYRA.md file cascade).
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
 *  the user's home. Shared by the memory files and the AGENTS.md docs. */
export type WorkspaceMemoryScope = "cwd" | "projectRoot" | "home";

export interface WorkspaceAgentDoc {
  path: string;
  title: string;
  scope: WorkspaceMemoryScope;
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

export interface WorkspaceMemoryQuery {
  cwd?: string;
}

export interface WorkspaceMemoryEntry {
  scope: WorkspaceMemoryScope;
  path: string;
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
export const WORKSPACE_SKILL_DRAFTS_KEY = "skill-drafts";
export const WORKSPACE_AGENT_MEMORY_KEY = "agent-memory";
export const WORKSPACE_MEMORY_KEY = "memory";
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
export const useWorkspaceSkills = createDataQuery<WorkspaceSkill[]>(WORKSPACE_SKILLS_KEY);
export const useManagedSkills = createDataQuery<ManagedSkill[]>(WORKSPACE_MANAGED_SKILLS_KEY);
export const useSkillDrafts = createDataQuery<SkillDraft[]>(WORKSPACE_SKILL_DRAFTS_KEY);
export const useAgentMemory = createParameterizedDataQuery<AgentMemoryQuery, AgentMemoryEntry[]>(
  WORKSPACE_AGENT_MEMORY_KEY,
);
export const useWorkspaceMemory = createParameterizedDataQuery<
  WorkspaceMemoryQuery,
  WorkspaceMemoryEntry[]
>(WORKSPACE_MEMORY_KEY);
export const useWorkspaceAgentDocs = createDataQuery<WorkspaceAgentDoc[]>(WORKSPACE_AGENT_DOCS_KEY);
export const useWorkspaceListFiles = createParameterizedDataQuery<
  WorkspaceListFilesQuery,
  WorkspaceFileEntry[]
>(WORKSPACE_LIST_FILES_KEY);
export const useWorkspaceReadFile = createParameterizedDataQuery<
  WorkspaceReadFileQuery,
  WorkspaceFileContent
>(WORKSPACE_READ_FILE_KEY);
export const useWorkspaceRecipes = useRecipes;
