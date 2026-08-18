import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileFocus } from "@/plugins/builtin/workspace/public/navigation";
import { isVcsUnavailable } from "./vcsAvailability";
import type { WorkspaceDiff, WorkspaceDiffQuery, WorkspaceFileDiff } from "./workspaceQueries";
import { useWorkspaceDiff } from "./workspaceQueries";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export type WorkspaceDiffMode = NonNullable<WorkspaceDiffQuery["mode"]>;
export type { WorkspaceFileDiff } from "./workspaceQueries";

export interface WorkspaceDiffSubtext {
  added: number;
  removed: number;
  fileCount: number;
}

export interface WorkspaceDiffViewModel {
  files?: WorkspaceFileDiff[];
  subtext?: WorkspaceDiffSubtext;
  truncated: boolean;
}

export interface WorkspaceDiffFileHeader {
  path: string;
  /** Set only for a rename: where the file came from. */
  previousPath?: string;
  added?: number;
  removed?: number;
}

/**
 * The review panel's read model: the whole comparison, plus which file the
 * review is focused on.
 *
 * The query is deliberately NOT scoped by the active file. A reviewer needs the
 * change as a whole — the active file is where to look first, not what to look
 * at, and the panel scrolls to it rather than filtering down to it.
 */
export function useWorkspaceDiffView(mode: WorkspaceDiffMode) {
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const fileFocus = useWorkspaceFileFocus();
  const query = useWorkspaceDiff(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd, mode } : undefined,
  );
  const view = workspaceDiffViewModel(query.data);
  return {
    fileFocus,
    data: query.data,
    files: view.files,
    isLoading: query.isLoading || workspace.status === "resolving",
    isError: query.isError,
    gitEnabled,
    notARepo: isVcsUnavailable(query.error),
    view,
  };
}

export function workspaceDiffViewModel(data: WorkspaceDiff | undefined): WorkspaceDiffViewModel {
  const files = data?.files;
  if (!files) return { truncated: false };

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
  };
}

/**
 * The paths and figures a file's card announces itself with.
 *
 * The two paths stay distinct so the view can truncate each around its filename
 * and allocate space without parsing a presentation string.
 */
export function workspaceDiffFileHeader(file: WorkspaceFileDiff): WorkspaceDiffFileHeader {
  return {
    path: file.path,
    ...(file.previousPath ? { previousPath: file.previousPath } : {}),
    added: file.added,
    removed: file.removed,
  };
}
