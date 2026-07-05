import type {
  DiffRow,
  FileChange,
  FileEntryInfo,
  GrepMatch,
  MemoryEntryInfo,
  WorkspaceAgentDoc as WorkspaceAgentDocInfo,
  WorkspaceSkill as WorkspaceSkillInfo,
} from "@/lib/data/queries";
import {
  useAgentDocs,
  useFilesChanged,
  useGrep,
  useListFiles,
  useReadFile,
  useRecipes,
  useSkills,
} from "@/lib/data/queries";

export type WorkspaceDiffRow = DiffRow;
export type WorkspaceFileChange = FileChange;
export type WorkspaceFileEntry = FileEntryInfo;
export type WorkspaceGrepMatch = GrepMatch;
export type WorkspaceAgentDoc = WorkspaceAgentDocInfo;
export type WorkspaceSkill = WorkspaceSkillInfo;
export type WorkspaceScope = MemoryEntryInfo["scope"];

export const useWorkspaceAgentDocs = useAgentDocs;
export const useWorkspaceFileChanges = useFilesChanged;
export const useWorkspaceGrep = useGrep;
export const useWorkspaceListFiles = useListFiles;
export const useWorkspaceReadFile = useReadFile;
export const useWorkspaceRecipes = useRecipes;
export const useWorkspaceSkills = useSkills;
