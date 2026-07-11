import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useActiveWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import { isVcsUnavailable } from "./vcsAvailability";
import type { WorkspaceDiff, WorkspaceDiffQuery, WorkspaceFileDiff } from "./workspaceData";
import { useWorkspaceDiff } from "./workspaceData";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export type WorkspaceDiffMode = NonNullable<WorkspaceDiffQuery["mode"]>;
export type { WorkspaceFileDiff } from "./workspaceData";

export interface WorkspaceDiffSubtext {
  added: number;
  removed: number;
  fileCount: number;
}

export interface WorkspaceDiffViewModel {
  files?: WorkspaceFileDiff[];
  subtext?: WorkspaceDiffSubtext;
  truncated: boolean;
  shouldShowFileHeaders: boolean;
}

export interface WorkspaceDiffFileHeader {
  displayPath: string;
  added?: number;
  removed?: number;
}

export function useWorkspaceDiffView(mode: WorkspaceDiffMode) {
  const gitEnabled = useWorkspaceCapability("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useActiveWorkspaceFile();
  const query = useWorkspaceDiff(
    gitEnabled ? { cwd, mode, path: activeFile || undefined } : undefined,
  );
  const view = workspaceDiffViewModel(query.data);
  return {
    activeFile,
    data: query.data,
    files: view.files,
    isLoading: query.isLoading,
    isError: query.isError,
    gitEnabled,
    notARepo: isVcsUnavailable(query.error),
    view,
  };
}

export function workspaceDiffViewModel(data: WorkspaceDiff | undefined): WorkspaceDiffViewModel {
  const files = data?.files;
  if (!files) {
    return {
      truncated: false,
      shouldShowFileHeaders: false,
    };
  }

  let added = 0;
  let removed = 0;
  for (const file of files) {
    added += file.added ?? 0;
    removed += file.removed ?? 0;
  }

  return {
    files,
    subtext: {
      added,
      removed,
      fileCount: files.length,
    },
    truncated: data.truncated ?? false,
    shouldShowFileHeaders: files.length > 1,
  };
}

export function workspaceDiffFileHeader(file: WorkspaceFileDiff): WorkspaceDiffFileHeader {
  return {
    displayPath: file.previousPath ? `${file.previousPath} → ${file.path}` : file.path,
    added: file.added,
    removed: file.removed,
  };
}
