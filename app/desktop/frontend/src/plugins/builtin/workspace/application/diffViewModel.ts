import type { DiffQuery, FileDiff } from "@/lib/data/queries";
import { useDiff } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useActiveWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import { isVcsUnavailable } from "./vcsAvailability";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export type WorkspaceDiffMode = NonNullable<DiffQuery["mode"]>;
export type WorkspaceFileDiff = FileDiff;

export function useWorkspaceDiffView(mode: WorkspaceDiffMode) {
  const gitEnabled = useWorkspaceCapability("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useActiveWorkspaceFile();
  const query = useDiff(gitEnabled ? { cwd, mode, path: activeFile || undefined } : undefined);
  const files = query.data?.files;
  return {
    activeFile,
    data: query.data,
    files,
    isLoading: query.isLoading,
    isError: query.isError,
    gitEnabled,
    notARepo: isVcsUnavailable(query.error),
    added: files?.reduce((sum, file) => sum + (file.added ?? 0), 0) ?? 0,
    removed: files?.reduce((sum, file) => sum + (file.removed ?? 0), 0) ?? 0,
  };
}
