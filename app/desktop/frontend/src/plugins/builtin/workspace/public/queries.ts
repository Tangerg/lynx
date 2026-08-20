export {
  WORKSPACE_AGENT_DOCS_KEY,
  WORKSPACE_BUILTIN_TOOLS_KEY,
  WORKSPACE_DIFF_KEY,
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_FILE_HEAD_KEY,
  WORKSPACE_GREP_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_KNOWLEDGE_KEY,
  WORKSPACE_PROJECTS_KEY,
  WORKSPACE_READ_FILE_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  WORKSPACE_AGENT_MEMORY_KEY,
  useWorkspaceFileChanges,
  useWorkspaceFileHead,
  useWorkspaceGrep,
  useWorkspaceListFiles,
  useWorkspaceProjects,
} from "../application/workspaceQueries";
export {
  type WorkspaceDiff,
  type WorkspaceDiffQuery,
  type WorkspaceFileChange,
  type WorkspaceFileChangesQuery,
  type WorkspaceFileHeadQuery,
  type WorkspaceFileLine,
  type WorkspaceGrepQuery,
  type WorkspaceGrepResult,
  type WorkspaceListFilesQuery,
  type WorkspaceKnowledgeQuery,
  type WorkspaceCatalogQuery,
  type WorkspaceProjectSummary,
  type WorkspaceReadFileQuery,
  type AgentMemoryQuery,
} from "../application/workspaceQueries";

// Capability gating is part of the cross-context surface; consumers must not
// infer a workspace feature from connection phase or stale query material.
export { useWorkspaceCapability } from "../application/workspaceCapabilities";
